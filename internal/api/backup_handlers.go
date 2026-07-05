package api

import (
	"context"
	"dgsmgt/internal/models"
	"dgsmgt/internal/notify"
	"dgsmgt/internal/utils"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gorilla/mux"
)

func (a *API) backupAccess(w http.ResponseWriter, r *http.Request) (*models.Server, bool) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	perms, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return nil, false
	}
	if !perms.CanManageBackups {
		utils.Forbidden(w, "Permission denied: cannot manage backups")
		return nil, false
	}
	return server, true
}

func (a *API) ListBackupsHandler(w http.ResponseWriter, r *http.Request) {
	server, ok := a.backupAccess(w, r)
	if !ok {
		return
	}
	var backups []models.Backup
	if err := a.db.Where("server_id = ?", server.ID).Order("created_at desc").Find(&backups).Error; err != nil {
		a.internalError(w, r, err, "Failed to list backups")
		return
	}
	utils.Success(w, backups)
}

// CreateBackupHandler starts a backup in the background.
func (a *API) CreateBackupHandler(w http.ResponseWriter, r *http.Request) {
	server, ok := a.backupAccess(w, r)
	if !ok {
		return
	}
	claims := claimsFrom(r)
	var input struct {
		Note string `json:"note" validate:"omitempty,max=200"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}

	srv := *server
	go func() {
		rec, err := a.backups.Run(context.Background(), &srv, input.Note)
		level, title := "success", fmt.Sprintf("Backup completed for %s", srv.Name)
		body := "Manual backup finished."
		if err != nil {
			level, title = "error", fmt.Sprintf("Backup failed for %s", srv.Name)
			body = err.Error()
		} else {
			body = fmt.Sprintf("%s (%.1f MB)", rec.FileName, float64(rec.SizeBytes)/1024/1024)
		}
		a.notifier.Dispatch(notify.Event{
			Type: notify.EventBackup, ServerID: srv.ID, ServerName: srv.Name,
			Title: title, Body: body, Level: level,
		})
	}()

	a.audit(r, claims, "create_backup", auditOpts{Server: server, Details: "Started manual backup", Success: true})
	utils.Success(w, map[string]string{"status": "started"})
}

// RestoreBackupHandler restores a backup into the current container.
func (a *API) RestoreBackupHandler(w http.ResponseWriter, r *http.Request) {
	server, ok := a.backupAccess(w, r)
	if !ok {
		return
	}
	claims := claimsFrom(r)
	vars := mux.Vars(r)

	var rec models.Backup
	if err := a.db.Where("id = ? AND server_id = ?", vars["backupId"], server.ID).First(&rec).Error; err != nil {
		utils.NotFound(w, "Backup not found")
		return
	}
	if rec.Status != "done" {
		utils.BadRequest(w, "Backup is not in a restorable state")
		return
	}

	if err := a.backups.Restore(r.Context(), server, &rec); err != nil {
		a.audit(r, claims, "restore_backup", auditOpts{Server: server, Details: err.Error(), Success: false})
		utils.BadRequest(w, "Restore failed: "+err.Error())
		return
	}
	a.audit(r, claims, "restore_backup", auditOpts{Server: server, Details: "Restored " + rec.FileName, Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

// DownloadBackupHandler serves the backup archive.
func (a *API) DownloadBackupHandler(w http.ResponseWriter, r *http.Request) {
	server, ok := a.backupAccess(w, r)
	if !ok {
		return
	}
	vars := mux.Vars(r)
	var rec models.Backup
	if err := a.db.Where("id = ? AND server_id = ?", vars["backupId"], server.ID).First(&rec).Error; err != nil {
		utils.NotFound(w, "Backup not found")
		return
	}
	p := filepath.Join(a.cfg.BackupPath, filepath.Base(rec.FileName))
	if _, err := os.Stat(p); err != nil {
		utils.NotFound(w, "Backup file not available locally")
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, rec.FileName))
	http.ServeFile(w, r, p)
}

// DeleteBackupHandler removes a backup record + file.
func (a *API) DeleteBackupHandler(w http.ResponseWriter, r *http.Request) {
	server, ok := a.backupAccess(w, r)
	if !ok {
		return
	}
	claims := claimsFrom(r)
	vars := mux.Vars(r)
	var rec models.Backup
	if err := a.db.Where("id = ? AND server_id = ?", vars["backupId"], server.ID).First(&rec).Error; err != nil {
		utils.NotFound(w, "Backup not found")
		return
	}
	a.backups.DeleteBackup(&rec)
	a.audit(r, claims, "delete_backup", auditOpts{Server: server, Details: "Deleted " + rec.FileName, Success: true})
	w.WriteHeader(http.StatusNoContent)
}
