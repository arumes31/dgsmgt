// Package notify fans out panel events to in-app notifications, browser push
// and external channels (Discord, generic webhooks, Telegram, email, ntfy,
// Gotify).
package notify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	"dgsmgt/internal/config"
	"dgsmgt/internal/models"

	webpush "github.com/SherClockHolmes/webpush-go"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ChannelSettings is stored as JSON in settings key "notify_channels"
// (editable in the admin settings UI).
type ChannelSettings struct {
	DiscordWebhook   string `json:"discord_webhook"`
	GenericWebhook   string `json:"generic_webhook"`
	TelegramBotToken string `json:"telegram_bot_token"`
	TelegramChatID   string `json:"telegram_chat_id"`
	NtfyURL          string `json:"ntfy_url"` // e.g. https://ntfy.sh/mytopic
	GotifyURL        string `json:"gotify_url"`
	GotifyToken      string `json:"gotify_token"`
	EmailEnabled     bool   `json:"email_enabled"`
}

type EventType string

const (
	EventServerDown  EventType = "server_down"
	EventServerUp    EventType = "server_up"
	EventServerCrash EventType = "server_crash"
	EventBackup      EventType = "backup"
	EventLogin       EventType = "login"
	EventLogAlert    EventType = "log_alert"
)

type Event struct {
	Type       EventType
	ServerID   uint
	ServerName string
	Title      string
	Body       string
	Level      string // info|warn|error|success
}

type Notifier struct {
	db     *gorm.DB
	cfg    *config.Config
	logger *zap.Logger
	http   *http.Client

	vapidPublic  string
	vapidPrivate string
}

func New(db *gorm.DB, cfg *config.Config, logger *zap.Logger) *Notifier {
	n := &Notifier{
		db:     db,
		cfg:    cfg,
		logger: logger,
		http:   &http.Client{Timeout: 10 * time.Second},
	}
	n.ensureVAPIDKeys()
	return n
}

// VAPIDPublicKey exposes the public key for the frontend push subscription.
func (n *Notifier) VAPIDPublicKey() string { return n.vapidPublic }

func (n *Notifier) ensureVAPIDKeys() {
	var pub, priv models.Setting
	errPub := n.db.First(&pub, "key = ?", "vapid_public").Error
	errPriv := n.db.First(&priv, "key = ?", "vapid_private").Error
	if errPub == nil && errPriv == nil {
		n.vapidPublic, n.vapidPrivate = pub.Value, priv.Value
		return
	}
	privKey, pubKey, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		n.logger.Error("failed to generate VAPID keys", zap.Error(err))
		return
	}
	n.db.Save(&models.Setting{Key: "vapid_public", Value: pubKey})
	n.db.Save(&models.Setting{Key: "vapid_private", Value: privKey})
	n.vapidPublic, n.vapidPrivate = pubKey, privKey
}

func (n *Notifier) channels() ChannelSettings {
	var s models.Setting
	cs := ChannelSettings{}
	if err := n.db.First(&s, "key = ?", "notify_channels").Error; err == nil {
		_ = json.Unmarshal([]byte(s.Value), &cs)
	}
	return cs
}

// Dispatch fans an event out to all channels. Safe to call from goroutines.
func (n *Notifier) Dispatch(ev Event) {
	go n.dispatch(ev)
}

func (n *Notifier) dispatch(ev Event) {
	defer func() {
		if r := recover(); r != nil {
			n.logger.Error("notify dispatch panic", zap.Any("panic", r))
		}
	}()

	cs := n.channels()

	// External panel-wide channels
	if cs.DiscordWebhook != "" {
		n.sendDiscord(cs.DiscordWebhook, ev)
	}
	if cs.GenericWebhook != "" {
		n.sendGeneric(cs.GenericWebhook, ev)
	}
	if cs.TelegramBotToken != "" && cs.TelegramChatID != "" {
		n.sendTelegram(cs.TelegramBotToken, cs.TelegramChatID, ev)
	}
	if cs.NtfyURL != "" {
		n.sendNtfy(cs.NtfyURL, ev)
	}
	if cs.GotifyURL != "" && cs.GotifyToken != "" {
		n.sendGotify(cs.GotifyURL, cs.GotifyToken, ev)
	}

	// Per-user: in-app + push + email, honoring preferences and server access.
	users := n.recipientsFor(ev)
	for _, u := range users {
		notif := models.Notification{
			UserID:   u.ID,
			ServerID: ev.ServerID,
			Level:    ev.Level,
			Title:    ev.Title,
			Body:     ev.Body,
		}
		if err := n.db.Create(&notif).Error; err != nil {
			n.logger.Error("failed to store notification", zap.Error(err))
		}
		n.sendWebPush(u.ID, ev)
		if cs.EmailEnabled && u.Email != "" && n.cfg.SMTPHost != "" {
			n.SendEmail(u.Email, "[DGSMgt] "+ev.Title, ev.Body)
		}
	}
}

// recipientsFor returns users who should receive an event: admins plus users
// assigned to the server, filtered by their notification preferences.
func (n *Notifier) recipientsFor(ev Event) []models.User {
	var users []models.User
	if ev.ServerID == 0 {
		n.db.Where("is_admin = ? AND disabled = ?", true, false).Find(&users)
	} else {
		n.db.Distinct("users.*").
			Joins("LEFT JOIN user_servers ON user_servers.user_id = users.id").
			Where("users.disabled = ?", false).
			Where("users.is_admin = ? OR user_servers.server_id = ?", true, ev.ServerID).
			Find(&users)
	}

	out := make([]models.User, 0, len(users))
	for _, u := range users {
		if n.wantsEvent(u.ID, ev) {
			out = append(out, u)
		}
	}
	return out
}

func (n *Notifier) wantsEvent(userID uint, ev Event) bool {
	var pref models.NotificationPref
	// Server-specific pref wins over the global (server 0) pref.
	err := n.db.Where("user_id = ? AND server_id = ?", userID, ev.ServerID).First(&pref).Error
	if err != nil {
		err = n.db.Where("user_id = ? AND server_id = 0", userID).First(&pref).Error
	}
	if err != nil {
		// Default: down + crash only.
		pref = models.NotificationPref{OnDown: true, OnCrash: true}
	}
	switch ev.Type {
	case EventServerDown:
		return pref.OnDown
	case EventServerUp:
		return pref.OnUp
	case EventServerCrash, EventLogAlert:
		return pref.OnCrash
	case EventBackup:
		return pref.OnBackup
	case EventLogin:
		return pref.OnLogin
	}
	return true
}

func (n *Notifier) post(urlStr, contentType string, body []byte) {
	resp, err := n.http.Post(urlStr, contentType, bytes.NewReader(body))
	if err != nil {
		// url.Error.Error() embeds the full request URL (path + query), which
		// can carry bot tokens / webhook secrets — log only the underlying
		// transport error so it doesn't undo redact(urlStr).
		logErr := err
		var uerr *url.Error
		if errors.As(err, &uerr) && uerr.Err != nil {
			logErr = uerr.Err
		}
		n.logger.Warn("notification post failed", zap.String("url", redact(urlStr)), zap.Error(logErr))
		return
	}
	_ = resp.Body.Close()
}

func redact(u string) string {
	if p, err := url.Parse(u); err == nil {
		return p.Scheme + "://" + p.Host + "/..."
	}
	return "..."
}

func (n *Notifier) sendDiscord(webhook string, ev Event) {
	color := 0x5865F2
	switch ev.Level {
	case "error":
		color = 0xED4245
	case "warn":
		color = 0xFEE75C
	case "success":
		color = 0x57F287
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"embeds": []map[string]interface{}{{
			"title":       ev.Title,
			"description": ev.Body,
			"color":       color,
			"footer":      map[string]string{"text": "DGSMgt"},
			"timestamp":   time.Now().Format(time.RFC3339),
		}},
	})
	n.post(webhook, "application/json", payload)
}

func (n *Notifier) sendGeneric(webhook string, ev Event) {
	payload, _ := json.Marshal(map[string]interface{}{
		"type":        string(ev.Type),
		"server_id":   ev.ServerID,
		"server_name": ev.ServerName,
		"title":       ev.Title,
		"body":        ev.Body,
		"level":       ev.Level,
		"timestamp":   time.Now().Format(time.RFC3339),
	})
	n.post(webhook, "application/json", payload)
}

func (n *Notifier) sendTelegram(botToken, chatID string, ev Event) {
	api := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	payload, _ := json.Marshal(map[string]string{
		"chat_id":    chatID,
		"text":       fmt.Sprintf("*%s*\n%s", ev.Title, ev.Body),
		"parse_mode": "Markdown",
	})
	n.post(api, "application/json", payload)
}

func (n *Notifier) sendNtfy(topicURL string, ev Event) {
	req, err := http.NewRequest("POST", topicURL, strings.NewReader(ev.Body))
	if err != nil {
		return
	}
	req.Header.Set("Title", ev.Title)
	if ev.Level == "error" {
		req.Header.Set("Priority", "high")
	}
	resp, err := n.http.Do(req)
	if err != nil {
		n.logger.Warn("ntfy post failed", zap.Error(err))
		return
	}
	_ = resp.Body.Close()
}

func (n *Notifier) sendGotify(baseURL, token string, ev Event) {
	api := strings.TrimRight(baseURL, "/") + "/message?token=" + url.QueryEscape(token)
	prio := 4
	if ev.Level == "error" {
		prio = 8
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"title":    ev.Title,
		"message":  ev.Body,
		"priority": prio,
	})
	n.post(api, "application/json", payload)
}

func (n *Notifier) sendWebPush(userID uint, ev Event) {
	if n.vapidPrivate == "" {
		return
	}
	var subs []models.PushSubscription
	n.db.Where("user_id = ?", userID).Find(&subs)
	if len(subs) == 0 {
		return
	}
	payload, _ := json.Marshal(map[string]string{
		"title": ev.Title,
		"body":  ev.Body,
		"level": ev.Level,
	})
	for _, sub := range subs {
		s := &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
		}
		resp, err := webpush.SendNotification(payload, s, &webpush.Options{
			Subscriber:      "mailto:admin@dgsmgt.local",
			VAPIDPublicKey:  n.vapidPublic,
			VAPIDPrivateKey: n.vapidPrivate,
			TTL:             300,
		})
		if err != nil {
			continue
		}
		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			n.db.Unscoped().Delete(&sub) // stale subscription
		}
		_ = resp.Body.Close()
	}
}

// SendEmail sends a plain-text email via configured SMTP.
func (n *Notifier) SendEmail(to, subject, body string) {
	if n.cfg.SMTPHost == "" {
		return
	}
	addr := n.cfg.SMTPHost + ":" + n.cfg.SMTPPort
	msg := []byte("From: " + n.cfg.SMTPFrom + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" +
		body + "\r\n")

	var auth smtp.Auth
	if n.cfg.SMTPUser != "" {
		auth = smtp.PlainAuth("", n.cfg.SMTPUser, n.cfg.SMTPPass, n.cfg.SMTPHost)
	}
	if err := smtp.SendMail(addr, auth, n.cfg.SMTPFrom, []string{to}, msg); err != nil {
		n.logger.Warn("email send failed", zap.String("to", to), zap.Error(err))
	}
}
