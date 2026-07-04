package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Discord OAuth2 login. Configured via DISCORD_CLIENT_ID / DISCORD_CLIENT_SECRET /
// DISCORD_REDIRECT_URL. Users can link their Discord account in the profile
// page, or (with DISCORD_AUTO_CREATE=true) accounts are provisioned on first login.

const discordAPI = "https://discord.com/api/v10"

type DiscordUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Avatar   string `json:"avatar"`
}

var discordHTTP = &http.Client{Timeout: 10 * time.Second}

// DiscordAuthURL builds the OAuth2 authorize URL.
func DiscordAuthURL(clientID, redirectURL, state string) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURL)
	q.Set("response_type", "code")
	q.Set("scope", "identify email")
	q.Set("state", state)
	return "https://discord.com/oauth2/authorize?" + q.Encode()
}

// DiscordExchangeCode exchanges an OAuth2 code for the Discord user identity.
var DiscordExchangeCode = func(clientID, clientSecret, redirectURL, code string) (*DiscordUser, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURL)

	resp, err := discordHTTP.Post(discordAPI+"/oauth2/token",
		"application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord token exchange failed: %s", resp.Status)
	}

	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, err
	}

	req, _ := http.NewRequest("GET", discordAPI+"/users/@me", nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	uresp, err := discordHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = uresp.Body.Close() }()
	if uresp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discord user fetch failed: %s", uresp.Status)
	}

	var du DiscordUser
	if err := json.NewDecoder(uresp.Body).Decode(&du); err != nil {
		return nil, err
	}
	return &du, nil
}
