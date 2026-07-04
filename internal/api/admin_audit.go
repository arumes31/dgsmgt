package api

import (
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"encoding/csv"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

// ---- Audit v2 ---------------------------------------------------------------------

// ListAuditLogsHandler supports filtering + pagination:
// ?user=&server_id=&action=&success=&from=&to=&page=&per_page=&format=csv
func (a *API) ListAuditLogsHandler(w http.ResponseWriter, r *http.Request) {
	q := a.db.Model(&models.AuditLog{})

	if v := r.URL.Query().Get("user"); v != "" {
		q = q.Where("username = ?", v)
	}
	if v := r.URL.Query().Get("server_id"); v != "" {
		q = q.Where("server_id = ?", v)
	}
	if v := r.URL.Query().Get("action"); v != "" {
		q = q.Where("action = ?", v)
	}
	if v := r.URL.Query().Get("success"); v != "" {
		q = q.Where("success = ?", v == "true")
	}
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q = q.Where("created_at <= ?", t)
		}
	}

	if r.URL.Query().Get("format") == "csv" {
		var logs []models.AuditLog
		q.Order("created_at desc").Limit(10000).Find(&logs)
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="audit-log.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"time", "username", "action", "server", "details", "ip", "success"})
		for _, l := range logs {
			_ = cw.Write([]string{l.CreatedAt.Format(time.RFC3339), l.Username, l.Action,
				l.ServerName, l.Details, l.IP, strconv.FormatBool(l.Success)})
		}
		cw.Flush()
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 || perPage > 200 {
		perPage = 50
	}

	var total int64
	q.Count(&total)

	var logs []models.AuditLog
	if err := q.Order("created_at desc").Offset((page - 1) * perPage).Limit(perPage).Find(&logs).Error; err != nil {
		a.internalError(w, r, err, "Failed to list audit logs")
		return
	}
	utils.JSON(w, http.StatusOK, logs, "", map[string]interface{}{
		"page": page, "per_page": perPage, "total": total,
	})
}

// ServerActivityHandler returns audit entries for one server (users with any
// access on it may view).
func (a *API) ServerActivityHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	_, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	var logs []models.AuditLog
	if err := a.db.Where("server_id = ?", server.ID).Order("created_at desc").Limit(100).Find(&logs).Error; err != nil {
		a.internalError(w, r, err, "Failed to list activity")
		return
	}
	utils.Success(w, logs)
}
