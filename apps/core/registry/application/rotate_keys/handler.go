// Package rotate_keys is the write-side use case: mint an application a new
// pair while its old one keeps working for the grace period.
package rotate_keys

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
	a, err := h.repo.Get(ctx, cmd.Name)
	if errors.Is(err, infrastructure.ErrNotFound) {
		return Result{}, fmt.Errorf("%w: %s", cqrs.ErrNotFound, cmd.Name)
	}
	if err != nil {
		return Result{}, err
	}
	if err := a.Rotate(h.now().UTC()); err != nil {
		return Result{}, fmt.Errorf("%w: %v", cqrs.ErrValidation, err)
	}
	if err := h.repo.Save(ctx, a); err != nil {
		return Result{}, err
	}
	return Result{App: a}, nil
}
