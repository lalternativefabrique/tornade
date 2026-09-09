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
// creation or rotation, and never listed.
type View struct {
	Name      string
	Active    bool
	CreatedAt time.Time
	RotatedAt *time.Time
	RevokedAt *time.Time
	// GraceUntil is when the previous pair stops working after a rotation.
	GraceUntil *time.Time
}

type Result struct {
	Apps []View
}

type Handler struct {
	repo *infrastructure.Repository
}

func NewHandler(repo *infrastructure.Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) Handle(ctx context.Context, _ Query) (Result, error) {
	apps, err := h.repo.List(ctx)
	if err != nil {
		return Result{}, err
	}
	out := Result{Apps: make([]View, 0, len(apps))}
	for _, a := range apps {
		v := View{Name: a.Name, Active: a.Active(), CreatedAt: a.CreatedAt, RotatedAt: a.RotatedAt, RevokedAt: a.RevokedAt}
		if a.RotatedAt != nil && a.Active() {
			until := a.RotatedAt.Add(24 * time.Hour)
			v.GraceUntil = &until
		}
		out.Apps = append(out.Apps, v)
	}
	return out, nil
}
