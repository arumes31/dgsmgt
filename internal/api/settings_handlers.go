package api

import (
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

// Editable panel settings (stored as key/value rows). Only these keys are
// exposed through the API.
var editableSettings = map[string]bool{
	"maintenance_message": true,
	"notify_channels":     true,
	"template_url":        true,
}

// PublicSettingsHandler exposes non-sensitive settings to any logged-in user.
func (a *API) PublicSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var s models.Setting
	msg := ""
	if err := a.db.First(&s, "key = ?", "maintenance_message").Error; err == nil {
		msg = s.Value
	}
	utils.Success(w, map[string]string{
		"maintenance_message": msg,
		"vapid_public_key":    a.notifier.VAPIDPublicKey(),
		"version":             a.cfg.Version,
	})
}

func (a *API) GetSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var settings []models.Setting
	a.db.Find(&settings)
	out := map[string]string{}
	for _, s := range settings {
		if editableSettings[s.Key] {
			out[s.Key] = s.Value
		}
	}
	utils.Success(w, out)
}

func (a *API) SetSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var input map[string]string
	// A map cannot go through decodeAndValidate: validate.Struct rejects any
	// non-struct, so that path would 400 every settings update. Decode directly.
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		utils.BadRequest(w, "Invalid request body")
		return
	}
	for k, v := range input {
		if !editableSettings[k] {
			utils.BadRequest(w, "Unknown setting: "+k)
			return
		}
		if err := a.db.Save(&models.Setting{Key: k, Value: v}).Error; err != nil {
			a.internalError(w, r, err, "Failed to save setting")
			return
		}
	}
	a.audit(r, claimsFrom(r), "update_settings", auditOpts{Details: "Updated panel settings", Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

// ---- Nodes ------------------------------------------------------------------------

func (a *API) ListNodesHandler(w http.ResponseWriter, r *http.Request) {
	var nodes []models.Node
	a.db.Find(&nodes)

	type nodeView struct {
		models.Node
		Online bool `json:"online"`
	}
	out := []nodeView{{
		Node:   models.Node{Name: "local", Address: "local docker socket", Enabled: true},
		Online: a.nodes.Local().Ping(r.Context()) == nil,
	}}
	for _, n := range nodes {
		nv := nodeView{Node: n}
		if svc, err := a.nodes.For(n.ID); err == nil {
			nv.Online = svc.Ping(r.Context()) == nil
		}
		out = append(out, nv)
	}
	utils.Success(w, out)
}

func (a *API) CreateNodeHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name    string `json:"name" validate:"required,min=2,max=64"`
		Address string `json:"address" validate:"required"`
		TLSCA   string `json:"tls_ca"`
		TLSCert string `json:"tls_cert"`
		TLSKey  string `json:"tls_key"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	node := models.Node{
		Name: input.Name, Address: input.Address,
		TLSCA: input.TLSCA, TLSCert: input.TLSCert, TLSKey: input.TLSKey,
		Enabled: true,
	}
	if err := a.db.Create(&node).Error; err != nil {
		utils.BadRequest(w, "Node name already exists")
		return
	}
	if err := a.nodes.Register(node); err != nil {
		a.db.Unscoped().Delete(&node)
		utils.BadRequest(w, "Cannot connect to node: "+err.Error())
		return
	}
	a.audit(r, claimsFrom(r), "create_node", auditOpts{Details: "Added node " + node.Name, Success: true})
	utils.Created(w, node)
}

func (a *API) DeleteNodeHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	nid, _ := strconv.Atoi(id)

	var count int64
	a.db.Model(&models.Server{}).Where("node_id = ?", nid).Count(&count)
	if count > 0 {
		utils.BadRequest(w, "Node still has servers assigned")
		return
	}

	a.nodes.Remove(uint(nid))
	if err := a.db.Unscoped().Delete(&models.Node{}, nid).Error; err != nil {
		a.internalError(w, r, err, "Failed to delete node")
		return
	}
	a.audit(r, claimsFrom(r), "delete_node", auditOpts{Details: "Removed node " + id, Success: true})
	w.WriteHeader(http.StatusNoContent)
}

// ---- Notification center ------------------------------------------------------------

func (a *API) ListNotificationsHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var notifs []models.Notification
	if err := a.db.Where("user_id = ?", claims.UserID).
		Order("created_at desc").Limit(50).Find(&notifs).Error; err != nil {
		a.internalError(w, r, err, "Failed to list notifications")
		return
	}
	var unread int64
	a.db.Model(&models.Notification{}).Where("user_id = ? AND read_at IS NULL", claims.UserID).Count(&unread)
	utils.JSON(w, http.StatusOK, notifs, "", map[string]interface{}{"unread": unread})
}

func (a *API) MarkNotificationsReadHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	now := time.Now()
	a.db.Model(&models.Notification{}).
		Where("user_id = ? AND read_at IS NULL", claims.UserID).Update("read_at", &now)
	utils.Success(w, map[string]string{"status": "ok"})
}

// ---- Notification preferences --------------------------------------------------------

func (a *API) GetNotificationPrefsHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var prefs []models.NotificationPref
	a.db.Where("user_id = ?", claims.UserID).Find(&prefs)
	utils.Success(w, prefs)
}

func (a *API) SetNotificationPrefHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var input struct {
		ServerID uint `json:"server_id"`
		OnDown   bool `json:"on_down"`
		OnUp     bool `json:"on_up"`
		OnCrash  bool `json:"on_crash"`
		OnBackup bool `json:"on_backup"`
		OnLogin  bool `json:"on_login"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	pref := models.NotificationPref{
		UserID: claims.UserID, ServerID: input.ServerID,
		OnDown: input.OnDown, OnUp: input.OnUp, OnCrash: input.OnCrash,
		OnBackup: input.OnBackup, OnLogin: input.OnLogin,
	}
	if err := a.db.Save(&pref).Error; err != nil {
		a.internalError(w, r, err, "Failed to save preference")
		return
	}
	utils.Success(w, pref)
}

// ---- Web push subscriptions -----------------------------------------------------------

func (a *API) PushSubscribeHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var input struct {
		Endpoint string `json:"endpoint" validate:"required,url"`
		P256dh   string `json:"p256dh" validate:"required"`
		Auth     string `json:"auth" validate:"required"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	sub := models.PushSubscription{
		UserID: claims.UserID, Endpoint: input.Endpoint,
		P256dh: input.P256dh, Auth: input.Auth,
	}
	// Upsert on endpoint
	a.db.Unscoped().Where("endpoint = ?", input.Endpoint).Delete(&models.PushSubscription{})
	if err := a.db.Create(&sub).Error; err != nil {
		a.internalError(w, r, err, "Failed to save subscription")
		return
	}
	utils.Created(w, map[string]string{"status": "ok"})
}

func (a *API) PushUnsubscribeHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var input struct {
		Endpoint string `json:"endpoint" validate:"required"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	a.db.Unscoped().Where("user_id = ? AND endpoint = ?", claims.UserID, input.Endpoint).
		Delete(&models.PushSubscription{})
	utils.Success(w, map[string]string{"status": "ok"})
}
