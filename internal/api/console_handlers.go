package api

import (
	"context"
	"dgsmgt/internal/models"
	"dgsmgt/internal/rcon"
	"dgsmgt/internal/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// ---- Interactive console -----------------------------------------------------------

// ConsoleHandler is a bidirectional websocket console: output frames are the
// same JSON format as LogsHandler; text messages from the client are sent to
// the server via RCON or container stdin depending on console_mode.
func (a *API) ConsoleHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	perms, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	if !perms.CanViewLogs {
		utils.Forbidden(w, "Permission denied")
		return
	}

	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	svc := mustSvc(a, server)
	reader, err := svc.Logs(r.Context(), id, "100", false)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"stream":"stderr","data":"Error attaching to logs"}`))
		return
	}
	defer func() { _ = reader.Close() }()

	// Input loop
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			cmd := string(msg)
			if cmd == "" {
				continue
			}
			if !perms.CanSendCommands {
				_ = conn.WriteMessage(websocket.TextMessage,
					[]byte(`{"stream":"stderr","data":"Permission denied: cannot send commands\n"}`))
				continue
			}
			out, err := a.sendCommand(server, cmd)
			a.audit(r, claims, "console_command", auditOpts{Server: server, Details: cmd, Success: err == nil})
			if err != nil {
				frame, _ := json.Marshal(map[string]string{"stream": "stderr", "data": "Command failed: " + err.Error() + "\n"})
				_ = conn.WriteMessage(websocket.TextMessage, frame)
			} else if out != "" {
				frame, _ := json.Marshal(map[string]string{"stream": "stdout", "data": out + "\n"})
				_ = conn.WriteMessage(websocket.TextMessage, frame)
			}
		}
	}()

	streamDemuxed(conn, reader)
}

func (a *API) sendCommand(server *models.Server, cmd string) (string, error) {
	switch server.ConsoleMode {
	case "none":
		return "", fmt.Errorf("console input disabled for this server")
	case "rcon":
		if server.RconHost == "" || server.RconPort == 0 {
			return "", fmt.Errorf("RCON not configured")
		}
		return rcon.Send(server.RconHost, server.RconPort, server.RconPassword, cmd)
	default: // attach (stdin)
		svc := mustSvc(a, server)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return "", svc.SendStdin(ctx, server.ContainerID, cmd)
	}
}

// CommandHandler sends a single command over HTTP (used by snippets/automation).
func (a *API) CommandHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	perms, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	if !perms.CanSendCommands {
		utils.Forbidden(w, "Permission denied: cannot send commands")
		return
	}
	var input struct {
		Command string `json:"command" validate:"required,max=1000"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	out, err := a.sendCommand(server, input.Command)
	a.audit(r, claims, "console_command", auditOpts{Server: server, Details: input.Command, Success: err == nil})
	if err != nil {
		utils.BadRequest(w, err.Error())
		return
	}
	utils.Success(w, map[string]string{"output": out})
}

// ---- Command snippets ---------------------------------------------------------------

func (a *API) ListSnippetsHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	_, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	var snippets []models.CommandSnippet
	a.db.Where("server_id = ?", server.ID).Find(&snippets)
	utils.Success(w, snippets)
}

func (a *API) CreateSnippetHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	perms, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	if !perms.CanSendCommands {
		utils.Forbidden(w, "Permission denied")
		return
	}
	var input struct {
		Name    string `json:"name" validate:"required,max=64"`
		Command string `json:"command" validate:"required,max=1000"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	snippet := models.CommandSnippet{ServerID: server.ID, Name: input.Name, Command: input.Command}
	if err := a.db.Create(&snippet).Error; err != nil {
		a.internalError(w, r, err, "Failed to save snippet")
		return
	}
	utils.Created(w, snippet)
}

func (a *API) DeleteSnippetHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	claims := claimsFrom(r)
	perms, server, status := a.getAccess(claims, vars["id"])
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	if !perms.CanSendCommands {
		utils.Forbidden(w, "Permission denied")
		return
	}
	a.db.Where("id = ? AND server_id = ?", vars["snippetId"], server.ID).Delete(&models.CommandSnippet{})
	w.WriteHeader(http.StatusNoContent)
}

// ---- Log alerts ------------------------------------------------------------------------

func (a *API) ListLogAlertsHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	_, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	var alerts []models.LogAlert
	a.db.Where("server_id = ?", server.ID).Find(&alerts)
	utils.Success(w, alerts)
}

func (a *API) CreateLogAlertHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	perms, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	if !perms.CanViewLogs {
		utils.Forbidden(w, "Permission denied")
		return
	}
	var input struct {
		Pattern string `json:"pattern" validate:"required,max=200"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	alert := models.LogAlert{ServerID: server.ID, Pattern: input.Pattern, Enabled: true}
	if err := a.db.Create(&alert).Error; err != nil {
		a.internalError(w, r, err, "Failed to save alert")
		return
	}
	a.audit(r, claims, "create_log_alert", auditOpts{Server: server, Details: input.Pattern, Success: true})
	utils.Created(w, alert)
}

func (a *API) DeleteLogAlertHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	claims := claimsFrom(r)
	_, server, status := a.getAccess(claims, vars["id"])
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	a.db.Where("id = ? AND server_id = ?", vars["alertId"], server.ID).Delete(&models.LogAlert{})
	w.WriteHeader(http.StatusNoContent)
}
