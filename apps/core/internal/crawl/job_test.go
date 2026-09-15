package crawl

import (
	"context"
	"strings"
	"testing"
	"time"
)

func runService(t *testing.T) (*Service, context.CancelFunc) {
	t.Helper()
	s := &Service{Store: NewMemoryStore(), Queue: NewMemoryQueue(), Fetcher: staticFetcher{}, MaxRunes: 6000}
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	return s, cancel
}

func waitDone(t *testing.T, s *Service, id string) Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		job, err := s.Store.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == StatusDone || job.Status == StatusFailed {
			return job
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job never finished")
	return Job{}
}

func TestServiceRunsASubmittedJobAndStoresItsPages(t *testing.T) {
	srv := site(t)
	s, cancel := runService(t)
	defer cancel()

	scope := Scope{Start: srv.URL + "/", MaxDepth: 1}
	if err := scope.Normalize(); err != nil {
		t.Fatal(err)
	}
	job, err := s.Submit(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != StatusQueued {
		t.Errorf("status = %s, want queued", job.Status)
	}

	job = waitDone(t, s, job.ID)
	if job.Status != StatusDone || job.Pages != 4 || job.Failed != 0 {
		t.Errorf("job = %+v, want done with 4 pages", job)
	}
	pages, err := s.Store.Pages(context.Background(), job.ID, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 4 || !strings.HasSuffix(pages[0].URL, "/") || pages[0].Title != "/" {
		t.Errorf("pages = %+v", pages)
	}
	second, _ := s.Store.Pages(context.Background(), job.ID, 2, 1)
	if len(second) != 1 {
		t.Errorf("offset/limit not honoured: %+v", second)
	}
}

func TestMapListsURLsUpToLimit(t *testing.T) {
	srv := site(t)
	scope := Scope{Start: srv.URL + "/", MaxDepth: 3, MaxPages: 50}
	if err := scope.Normalize(); err != nil {
		t.Fatal(err)
	}
	all := Map(context.Background(), scope, staticFetcher{}, 100)
	want := []string{"/", "/a", "/b", "/docs/x", "/private", "/a/1", "/docs/y", "/a/1/deep"}
	if len(all) != len(want) {
		t.Errorf("mapped %v, want %d urls", all, len(want))
	}
	if !strings.HasSuffix(all[0], "/") || !strings.HasSuffix(all[1], "/a") {
		t.Errorf("discovery order lost: %v", all)
	}
	few := Map(context.Background(), scope, staticFetcher{}, 3)
	if len(few) != 3 {
		t.Errorf("limit not honoured: %v", few)
	}
}
