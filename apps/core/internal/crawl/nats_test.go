package crawl

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func startNATS(t *testing.T) *nats.Conn {
	t.Helper()
	srv, err := server.NewServer(&server.Options{Port: -1, JetStream: true, StoreDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		t.Fatal("nats server not ready")
	}
	t.Cleanup(srv.Shutdown)
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	return nc
}

func TestNATSStoreRoundTripsJobsAndPages(t *testing.T) {
	nc := startNATS(t)
	store, err := NewNATSStore(nc, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := store.Get(ctx, "missing"); err != ErrNotFound {
		t.Errorf("Get missing = %v, want ErrNotFound", err)
	}
	job := Job{ID: "j1", Status: StatusQueued, Scope: Scope{Start: "https://example.com/"}}
	if err := store.Put(ctx, job); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(ctx, "j1")
	if err != nil || got.Scope.Start != job.Scope.Start {
		t.Errorf("Get = %+v, %v", got, err)
	}
	for i := 0; i < 5; i++ {
		if err := store.AddPage(ctx, "j1", i, StoredPage{URL: "https://example.com/" + string(rune('a'+i))}); err != nil {
			t.Fatal(err)
		}
	}
	pages, err := store.Pages(ctx, "j1", 3, 10)
	if err != nil || len(pages) != 2 || pages[0].URL != "https://example.com/d" {
		t.Errorf("Pages = %+v, %v", pages, err)
	}
}

func TestNATSQueueDeliversAndRedeliversOnFailure(t *testing.T) {
	nc := startNATS(t)
	queue, err := NewNATSQueue(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	deliveries := make(chan string, 8)
	attempts := 0
	go queue.Consume(ctx, func(_ context.Context, id string) error {
		attempts++
		deliveries <- id
		if attempts == 1 {
			return context.DeadlineExceeded
		}
		return nil
	})
	if err := queue.Enqueue(ctx, "job-1"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case id := <-deliveries:
			if id != "job-1" {
				t.Fatalf("delivered %q", id)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("delivery %d never came", i+1)
		}
	}
}

func TestServiceRunsOverNATS(t *testing.T) {
	nc := startNATS(t)
	store, err := NewNATSStore(nc, 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	queue, err := NewNATSQueue(nc)
	if err != nil {
		t.Fatal(err)
	}
	site := site(t)
	s := &Service{Store: store, Queue: queue, Fetcher: staticFetcher{}, MaxRunes: 6000}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	scope := Scope{Start: site.URL + "/", MaxDepth: 1}
	if err := scope.Normalize(); err != nil {
		t.Fatal(err)
	}
	job, err := s.Submit(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	job = waitDone(t, s, job.ID)
	if job.Status != StatusDone || job.Pages != 4 {
		t.Errorf("job = %+v", job)
	}
}
