// Package domain holds the App aggregate: one application allowed to speak
// through tornade, named by the issuer its signatures carry.
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Grace is how long a rotated-out pair keeps working. An application
// redeploys with its new keys inside it; URLs it signed before still play.
const Grace = 24 * time.Hour

var (
	ErrInvalidName = errors.New("registry: name must be kebab-case, [a-z][a-z0-9-]*")
	ErrRevoked     = errors.New("registry: app is revoked")
)

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// App is an application registered with tornade. SigningKey signs the URLs
// its browsers fetch audio on; AppKey authenticates its own server calls.
type App struct {
	Name               string
	SigningKey         string
	AppKey             string
	PreviousSigningKey string
	PreviousAppKey     string
	RotatedAt          *time.Time
	RevokedAt          *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Register mints a new application with a fresh pair of keys.
func Register(name string, now time.Time) (*App, error) {
	if !nameRe.MatchString(name) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return &App{
		Name:       name,
		SigningKey: newKey(),
		AppKey:     newKey(),
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// Rotate mints a new pair and keeps the old one valid for Grace.
func (a *App) Rotate(now time.Time) error {
	if a.RevokedAt != nil {
		return ErrRevoked
	}
	a.PreviousSigningKey, a.PreviousAppKey = a.SigningKey, a.AppKey
	a.SigningKey, a.AppKey = newKey(), newKey()
	a.RotatedAt = &now
	a.UpdatedAt = now
	return nil
}

// Revoke ends the application's access at once, previous pair included.
func (a *App) Revoke(now time.Time) {
	a.RevokedAt = &now
	a.UpdatedAt = now
}

// Active reports whether the application may still speak.
func (a *App) Active() bool { return a.RevokedAt == nil }

// SigningKeys are the secrets a signature from this application may verify
// against at now: the current one, and the previous one while its grace lasts.
func (a *App) SigningKeys(now time.Time) []string {
	return a.keys(now, a.SigningKey, a.PreviousSigningKey)
}

// AppKeys are the secrets this application may present on its own calls at now.
func (a *App) AppKeys(now time.Time) []string {
	return a.keys(now, a.AppKey, a.PreviousAppKey)
}

func (a *App) keys(now time.Time, current, previous string) []string {
	if !a.Active() {
		return nil
	}
	keys := []string{current}
	if previous != "" && a.RotatedAt != nil && now.Before(a.RotatedAt.Add(Grace)) {
		keys = append(keys, previous)
	}
	return keys
}

func newKey() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("registry: random source unavailable: %v", err))
	}
	return hex.EncodeToString(b)
}
