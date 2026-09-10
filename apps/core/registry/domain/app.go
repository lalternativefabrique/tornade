// Package domain holds the App aggregate: one application allowed to speak
// through tornade, named by the issuer its signatures carry. Its key, the one
// its server presents on its own calls and signs browser URLs with, is held
// sealed; only its last four characters are ever shown back.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Grace is how long a rotated-out key keeps working. An application
// redeploys with its new key inside it; URLs it signed before still play.
const Grace = 24 * time.Hour

var (
	ErrInvalidName = errors.New("registry: name must be kebab-case, [a-z][a-z0-9-]*")
	ErrRevoked     = errors.New("registry: app is revoked")
)

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// App is an application registered with tornade.
type App struct {
	Name      string
	Current   []byte
	Last4     string
	Previous  []byte
	RotatedAt *time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Register admits an application under a fresh key the caller sealed.
func Register(name string, sealed []byte, key string, now time.Time) (*App, error) {
	if !nameRe.MatchString(name) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	if len(sealed) == 0 {
		return nil, errors.New("registry: a sealed key is required")
	}
	return &App{
		Name:      name,
		Current:   sealed,
		Last4:     Last4(key),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Rotate installs a new key and keeps the old one valid for Grace.
func (a *App) Rotate(sealed []byte, key string, now time.Time) error {
	if a.RevokedAt != nil {
		return ErrRevoked
	}
	a.Previous = a.Current
	a.Current = sealed
	a.Last4 = Last4(key)
	a.RotatedAt = &now
	a.UpdatedAt = now
	return nil
}

// Revoke ends the application's access at once, previous key included.
func (a *App) Revoke(now time.Time) {
	a.RevokedAt = &now
	a.UpdatedAt = now
}

// Active reports whether the application may still speak.
func (a *App) Active() bool { return a.RevokedAt == nil }

// PreviousValidAt reports whether the rotated-out key still counts at now.
func (a *App) PreviousValidAt(now time.Time) bool {
	return a.Active() && a.RotatedAt != nil && len(a.Previous) > 0 && now.Before(a.RotatedAt.Add(Grace))
}

// GraceUntil is when the previous key stops working, nil when none is live.
func (a *App) GraceUntil(now time.Time) *time.Time {
	if !a.PreviousValidAt(now) {
		return nil
	}
	until := a.RotatedAt.Add(Grace)
	return &until
}

// Last4 is enough to tell two keys apart on screen without showing either.
func Last4(key string) string {
	if len(key) <= 4 {
		return key
	}
	return key[len(key)-4:]
}
