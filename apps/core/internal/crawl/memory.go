package crawl

import (
	"context"
	"sync"
)

// MemoryStore keeps jobs in this process, for a laptop with no broker and
// for tests. A restart forgets them.
type MemoryStore struct {
	mu    sync.Mutex
	jobs  map[string]Job
	pages map[string][]StoredPage
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{jobs: map[string]Job{}, pages: map[string][]StoredPage{}}
}

func (m *MemoryStore) Put(_ context.Context, job Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[job.ID] = job
	return nil
}

func (m *MemoryStore) Get(_ context.Context, id string) (Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return Job{}, ErrNotFound
	}
	return job, nil
}

func (m *MemoryStore) AddPage(_ context.Context, id string, seq int, page StoredPage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pages := m.pages[id]
	for len(pages) <= seq {
		pages = append(pages, StoredPage{})
	}
	pages[seq] = page
	m.pages[id] = pages
	return nil
}

func (m *MemoryStore) Pages(_ context.Context, id string, offset, limit int) ([]StoredPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	pages := m.pages[id]
	if offset >= len(pages) {
		return nil, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(pages) {
		end = len(pages)
	}
	return append([]StoredPage(nil), pages[offset:end]...), nil
}

// MemoryQueue hands jobs to the same process that accepted them.
type MemoryQueue struct {
	ids chan string
}

func NewMemoryQueue() *MemoryQueue {
	return &MemoryQueue{ids: make(chan string, 256)}
}

func (q *MemoryQueue) Enqueue(ctx context.Context, id string) error {
	select {
	case q.ids <- id:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *MemoryQueue) Consume(ctx context.Context, handle func(context.Context, string) error) error {
	for {
		select {
		case id := <-q.ids:
			if err := handle(ctx, id); err != nil && ctx.Err() == nil {
				select {
				case q.ids <- id:
				default:
				}
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
