// Package list_apps is the read-side use case: every application, keys
// withheld.
package list_apps

import (
	"context"
	"time"

	"github.com/lalternativefabrique/tornade/core/registry/infrastructure"
)

type Query struct{}

// View names an application without its secrets: those are shown once, at
// creation or rotation, and only their last four characters after that.
type View struct {
	Name         string
	Active       bool
	SigningLast4 string
	AppLast4     string
	CreatedAt    time.Time
	RotatedAt    *time.Time
	RevokedAt    *time.Time
	GraceUntil   *time.Time
}

type Result struct {
	Apps []View
}

type Handler struct {
	repo *infrastructure.Repository
	now  func() time.Time
}

func NewHandler(repo *infrastructure.Repository) *Handler {
	return &Handler{repo: repo, now: time.Now}
}

func (h *Handler) Handle(ctx context.Context, _ Query) (Result, error) {
	apps, err := h.repo.List(ctx)
	if err != nil {
		return Result{}, err
	}
	now := h.now().UTC()
	out := Result{Apps: make([]View, 0, len(apps))}
	for _, a := range apps {
		out.Apps = append(out.Apps, View{
			Name: a.Name, Active: a.Active(), SigningLast4: a.SigningLast4, AppLast4: a.AppLast4,
			CreatedAt: a.CreatedAt, RotatedAt: a.RotatedAt, RevokedAt: a.RevokedAt, GraceUntil: a.GraceUntil(now),
		})
	}
	return out, nil
}
