package api

import (
	"sync"
	"time"
)

const (
	loginLockoutWindow = 15 * time.Minute
	// Failed attempts allowed per IP and username before that pair is locked out
	maxFailuresPerIPUser = 5
	// Failed attempts allowed per IP across all usernames
	maxFailuresPerIP = 20
	// Failed attempts allowed per username across all IPs (bounds distributed guessing)
	maxFailuresPerUser = 50
)

type loginAttempt struct {
	count     int
	lastError time.Time
}

// loginLimiter tracks failed logins per IP, per username and per IP+username.
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*loginAttempt
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]*loginAttempt)}
}

func limiterKeys(ip, username string) []struct {
	key   string
	limit int
} {
	return []struct {
		key   string
		limit int
	}{
		{"ipuser:" + ip + "\x00" + username, maxFailuresPerIPUser},
		{"ip:" + ip, maxFailuresPerIP},
		{"user:" + username, maxFailuresPerUser},
	}
}

// Blocked reports whether a login for this IP and username is currently locked out.
func (l *loginLimiter) Blocked(ip, username string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for _, k := range limiterKeys(ip, username) {
		if a, ok := l.attempts[k.key]; ok && a.count >= k.limit && now.Sub(a.lastError) < loginLockoutWindow {
			return true
		}
	}
	return false
}

// Failure records a failed login.
func (l *loginLimiter) Failure(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.evictLocked(now)
	for _, k := range limiterKeys(ip, username) {
		a, ok := l.attempts[k.key]
		if !ok || now.Sub(a.lastError) >= loginLockoutWindow {
			a = &loginAttempt{}
			l.attempts[k.key] = a
		}
		a.count++
		a.lastError = now
	}
}

// Success clears the IP+username counter after a successful login.
func (l *loginLimiter) Success(ip, username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, limiterKeys(ip, username)[0].key)
}

func (l *loginLimiter) evictLocked(now time.Time) {
	for k, a := range l.attempts {
		if now.Sub(a.lastError) >= loginLockoutWindow {
			delete(l.attempts, k)
		}
	}
}
