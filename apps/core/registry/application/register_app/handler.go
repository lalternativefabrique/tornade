// Package register_app is the write-side use case: admit an application.
package register_app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lalternative/packages/go/eda/pkg/cqrs"

	"github.com/lalternativefabrique/tornade/core/registry/domain"
	"github.com/lalternativefabrique/tornade/core/registry/infrastructure"
)

type Command struct {
	Name string
}

// Result carries the keys once: they are shown to the operator on creation
// and never read back.
type Result struct {
	App *domain.App
}

type Handler struct {
	repo *infrastructure.Repository
	now  func() time.Time
}

func NewHandler(repo *infrastructure.Repository) *Handler {
	return &Handler{repo: repo, now: time.Now}
}

func (h *Handler) Handle(ctx context.Context, cmd Command) (Result, error) {
	if _, err := h.repo.Get(ctx, cmd.Name); err == nil {
		return Result{}, fmt.Errorf("%w: %s is already registered", cqrs.ErrValidation, cmd.Name)
	} else if !errors.Is(err, infrastructure.ErrNotFound) {
		return Result{}, err
	}
	a, err := domain.Register(cmd.Name, h.now().UTC())
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", cqrs.ErrValidation, err)
	}
	if err := h.repo.Save(ctx, a); err != nil {
		return Result{}, err
	}
	return Result{App: a}, nil
}
