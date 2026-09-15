package crawl

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"
)

const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Job is one crawl as a caller sees it: what was asked, where it stands,
// and how many pages it has produced so far.
type Job struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Scope     Scope     `json:"scope"`
	Pages     int       `json:"pages"`
	Failed    int       `json:"failed"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StoredPage is one page of a job as kept for the caller to read back.
type StoredPage struct {
	URL      string `json:"url"`
	Depth    int    `json:"depth"`
	Title    string `json:"title,omitempty"`
	Text     string `json:"text,omitempty"`
	Markdown string `json:"markdown,omitempty"`
	Error    string `json:"error,omitempty"`
}

var ErrNotFound = errors.New("crawl: job not found")

// Store keeps jobs and their pages where every replica can read them.
type Store interface {
	Put(ctx context.Context, job Job) error
	Get(ctx context.Context, id string) (Job, error)
	AddPage(ctx context.Context, id string, seq int, page StoredPage) error
	Pages(ctx context.Context, id string, offset, limit int) ([]StoredPage, error)
}

// Queue hands a submitted job to whichever replica picks it up. handle
// returning nil acknowledges the job; an error lets the queue redeliver it.
type Queue interface {
	Enqueue(ctx context.Context, id string) error
	Consume(ctx context.Context, handle func(ctx context.Context, id string) error) error
}

// Service is the crawl feature: it accepts jobs, runs them, and serves
// their state.
type Service struct {
	Store   Store
	Queue   Queue
	Fetcher Fetcher
	// MaxRunes bounds each rendering of a stored page.
	MaxRunes int
}

// Submit records the job and queues it. The scope must be normalized.
func (s *Service) Submit(ctx context.Context, scope Scope) (Job, error) {
	now := time.Now().UTC()
	job := Job{ID: newID(), Status: StatusQueued, Scope: scope, CreatedAt: now, UpdatedAt: now}
	if err := s.Store.Put(ctx, job); err != nil {
		return Job{}, err
	}
	if err := s.Queue.Enqueue(ctx, job.ID); err != nil {
		return Job{}, err
	}
	return job, nil
}

// Run consumes the queue until ctx ends. Every replica runs one.
func (s *Service) Run(ctx context.Context) error {
	return s.Queue.Consume(ctx, s.execute)
}

func (s *Service) execute(ctx context.Context, id string) error {
	job, err := s.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if job.Status == StatusDone {
		return nil
	}
	if err := job.Scope.Normalize(); err != nil {
		return s.finish(ctx, job, err)
	}
	job.Status = StatusRunning
	if err := s.touch(ctx, &job); err != nil {
		return err
	}

	seq := job.Pages + job.Failed
	var storeErr error
	Walk(ctx, job.Scope, s.Fetcher, func(r Result) {
		if storeErr != nil {
			return
		}
		page := StoredPage{URL: r.URL, Depth: r.Depth}
		if r.Err != nil {
			page.Error = r.Err.Error()
			job.Failed++
		} else {
			page.Title, page.Text, page.Markdown = r.Page.Title, r.Page.Text, r.Page.Markdown
			job.Pages++
		}
		if err := s.Store.AddPage(ctx, job.ID, seq, page); err != nil {
			storeErr = err
			return
		}
		seq++
		storeErr = s.touch(ctx, &job)
	})
	if storeErr != nil {
		return storeErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return s.finish(ctx, job, nil)
}

func (s *Service) touch(ctx context.Context, job *Job) error {
	job.UpdatedAt = time.Now().UTC()
	return s.Store.Put(ctx, *job)
}

func (s *Service) finish(ctx context.Context, job Job, cause error) error {
	job.Status = StatusDone
	if cause != nil {
		job.Status = StatusFailed
		job.Error = cause.Error()
		log.Printf("vvaves: crawl %s failed: %v", job.ID, cause)
	}
	return s.touch(ctx, &job)
}

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crawl: id: %v", err))
	}
	return hex.EncodeToString(b[:])
}
