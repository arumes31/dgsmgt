package api

import (
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// ---- Metrics -----------------------------------------------------------------------------

// MetricsHandler streams parsed stats snapshots over a websocket.
func (a *API) MetricsHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	_, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}

	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() { _ = conn.Close() }()

	svc := mustSvc(a, server)
	go drainWS(conn)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			snap, err := svc.StatsOnce(r.Context(), id)
			if err != nil {
				_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"stats unavailable"}`))
				return
			}
			frame, _ := json.Marshal(snap)
			if conn.WriteMessage(websocket.TextMessage, frame) != nil {
				return
			}
		}
	}
}

// MetricsHistoryHandler returns stored samples. Query: hours (default 24).
func (a *API) MetricsHistoryHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	_, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	hours, err := strconv.Atoi(r.URL.Query().Get("hours"))
	if err != nil || hours <= 0 || hours > 24*30 {
		hours = 24
	}
	var samples []models.MetricSample
	if err := a.db.Where("server_id = ? AND at > ?", server.ID, time.Now().Add(-time.Duration(hours)*time.Hour)).
		Order("at asc").Find(&samples).Error; err != nil {
		a.internalError(w, r, err, "Failed to load metrics")
		return
	}
	utils.Success(w, samples)
}

// AvailabilityHandler returns uptime percentage over the last N days (default 30).
func (a *API) AvailabilityHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	_, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	days, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || days <= 0 || days > 90 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days)

	var total, up int64
	a.db.Model(&models.MetricSample{}).Where("server_id = ? AND at > ?", server.ID, since).Count(&total)
	a.db.Model(&models.MetricSample{}).Where("server_id = ? AND at > ? AND running = ?", server.ID, since, true).Count(&up)

	pct := 0.0
	if total > 0 {
		pct = float64(up) / float64(total) * 100.0
	}
	utils.Success(w, map[string]interface{}{"days": days, "samples": total, "availability_percent": pct})
}
