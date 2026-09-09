// Package domain holds the App aggregate: one application allowed to speak
// through tornade, named by the issuer its signatures carry. Its two keys,
// the signing key its server signs browser URLs with and the app key it
// presents on its own calls, are held sealed; only their last four
// characters are ever shown back.
package domain

import (
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

// Keys is a pair in the clear. It exists only in the answer of the call that
// minted it.
type Keys struct {
	Signing string
	App     string
}

// Encrypted is a pair as stored.
type Encrypted struct {
	Signing []byte
	App     []byte
}

// App is an application registered with tornade.
type App struct {
	Name         string
	Current      Encrypted
	SigningLast4 string
	AppLast4     string
	Previous     Encrypted
	RotatedAt    *time.Time
	RevokedAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Register admits an application under a fresh pair the caller sealed.
func Register(name string, sealed Encrypted, keys Keys, now time.Time) (*App, error) {
	if !nameRe.MatchString(name) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	if len(sealed.Signing) == 0 || len(sealed.App) == 0 {
		return nil, errors.New("registry: sealed keys are required")
	}
	return &App{
		Name:         name,
		Current:      sealed,
		SigningLast4: Last4(keys.Signing),
		AppLast4:     Last4(keys.App),
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// Rotate installs a new pair and keeps the old one valid for Grace.
func (a *App) Rotate(sealed Encrypted, keys Keys, now time.Time) error {
	if a.RevokedAt != nil {
		return ErrRevoked
	}
	a.Previous = a.Current
	a.Current = sealed
	a.SigningLast4, a.AppLast4 = Last4(keys.Signing), Last4(keys.App)
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

// PreviousValidAt reports whether the rotated-out pair still counts at now.
func (a *App) PreviousValidAt(now time.Time) bool {
	return a.Active() && a.RotatedAt != nil && len(a.Previous.Signing) > 0 && now.Before(a.RotatedAt.Add(Grace))
}

// GraceUntil is when the previous pair stops working, nil when none is live.
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
