package api

import (
	"dgsmgt/internal/utils"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// ---- Live logs (websocket) ------------------------------------------------------

// LogsHandler streams container logs. Query params: tail (default 100),
// timestamps (true/false). Messages are JSON: {"stream":"stdout|stderr","data":"..."}.
func (a *API) LogsHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	perms, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	if !perms.CanViewLogs {
		utils.Forbidden(w, "Permission denied: cannot view logs")
		return
	}

	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	} else if _, err := strconv.Atoi(tail); err != nil && tail != "all" {
		tail = "100"
	}
	timestamps := r.URL.Query().Get("timestamps") == "true"

	a.audit(r, claims, "logs", auditOpts{Server: server, Details: "Viewed container logs", Success: true})

	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() { _ = conn.Close() }()

	svc := mustSvc(a, server)
	reader, err := svc.Logs(r.Context(), id, tail, timestamps)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"stream":"stderr","data":"Error reading logs"}`))
		return
	}
	defer func() { _ = reader.Close() }()

	go drainWS(conn)
	streamDemuxed(conn, reader)
}

// streamDemuxed forwards a multiplexed docker stream, preserving the
// stdout/stderr distinction as JSON frames.
func streamDemuxed(conn *websocket.Conn, reader io.Reader) {
	buf := make([]byte, 32*1024)
	pending := []byte{}
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			for {
				if len(pending) < 8 {
					break
				}
				streamType := pending[0]
				size := int(pending[4])<<24 | int(pending[5])<<16 | int(pending[6])<<8 | int(pending[7])
				if size < 0 || len(pending) < 8+size {
					// Not a valid header (TTY container) — send raw.
					if streamType != 1 && streamType != 2 && streamType != 0 {
						frame, _ := json.Marshal(map[string]string{"stream": "stdout", "data": string(pending)})
						if conn.WriteMessage(websocket.TextMessage, frame) != nil {
							return
						}
						pending = pending[:0]
					}
					break
				}
				name := "stdout"
				if streamType == 2 {
					name = "stderr"
				}
				frame, _ := json.Marshal(map[string]string{"stream": name, "data": string(pending[8 : 8+size])})
				if conn.WriteMessage(websocket.TextMessage, frame) != nil {
					return
				}
				pending = pending[8+size:]
			}
		}
		if err != nil {
			return
		}
	}
}

func drainWS(conn *websocket.Conn) {
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// DownloadLogsHandler returns recent logs as a downloadable text file.
func (a *API) DownloadLogsHandler(w http.ResponseWriter, r *http.Request) {
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

	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "5000"
	}
	svc := mustSvc(a, server)
	reader, err := svc.LogsOnce(r.Context(), id, tail)
	if err != nil {
		a.internalError(w, r, err, "Failed to fetch logs")
		return
	}
	defer func() { _ = reader.Close() }()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s-%s.log"`, server.Name, time.Now().Format("20060102-150405")))

	buf := make([]byte, 32*1024)
	pending := []byte{}
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			for len(pending) >= 8 {
				size := int(pending[4])<<24 | int(pending[5])<<16 | int(pending[6])<<8 | int(pending[7])
				if size < 0 || len(pending) < 8+size {
					break
				}
				_, _ = w.Write(pending[8 : 8+size])
				pending = pending[8+size:]
			}
		}
		if err != nil {
			if len(pending) > 0 && len(pending) < 8 {
				_, _ = w.Write(pending)
			}
			return
		}
	}
}

// PersistedLogHandler serves the archived log file for a server.
func (a *API) PersistedLogHandler(w http.ResponseWriter, r *http.Request) {
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
	p := filepath.Join(a.cfg.LogPath, filepath.Base(server.Name)+".log")
	if _, err := os.Stat(p); err != nil {
		utils.NotFound(w, "No persisted logs for this server")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-archive.log"`, server.Name))
	http.ServeFile(w, r, p)
}
