package pagecache

import (
	"testing"
	"time"

	"github.com/lalternative/packages/go/search"
	"github.com/lalternative/packages/go/search/fetch"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func startNATS(t *testing.T) *nats.Conn {
	t.Helper()
	srv, err := server.NewServer(&server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})
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

func TestGetReturnsWhatSetStored(t *testing.T) {
	nc := startNATS(t)
	c, err := NewNATS(nc, time.Minute, 1<<20)
	if err != nil {
		t.Fatal(err)
	}

	url := "https://example.com/article?id=1&lang=fr"
	stored := &fetch.Page{
		Title:     "Title",
		Text:      "Body",
		Favicon:   "https://example.com/favicon.ico",
		OpenGraph: &search.OpenGraph{Title: "OG", SiteName: "Example"},
	}
	c.Set(url, stored)

	got, ok := c.Get(url)
	if !ok {
		t.Fatal("expected a hit")
	}
	if got.Title != stored.Title || got.Text != stored.Text || got.Favicon != stored.Favicon {
		t.Errorf("got %+v, want %+v", got, stored)
	}
	if got.OpenGraph == nil || got.OpenGraph.SiteName != "Example" {
		t.Errorf("open graph not preserved: %+v", got.OpenGraph)
	}
}

func TestGetMissesAnUnknownURL(t *testing.T) {
	nc := startNATS(t)
	c, err := NewNATS(nc, time.Minute, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("https://example.com/never"); ok {
		t.Fatal("expected a miss")
	}
}

func TestEntriesExpireAfterTTL(t *testing.T) {
	nc := startNATS(t)
	c, err := NewNATS(nc, 200*time.Millisecond, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	c.Set("https://example.com/a", &fetch.Page{Title: "A"})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := c.Get("https://example.com/a"); !ok {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("entry still present after TTL")
}

func TestSecondOpenReusesTheBucket(t *testing.T) {
	nc := startNATS(t)
	first, err := NewNATS(nc, time.Minute, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	first.Set("https://example.com/shared", &fetch.Page{Title: "Shared"})

	second, err := NewNATS(nc, time.Minute, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := second.Get("https://example.com/shared"); !ok || got.Title != "Shared" {
		t.Fatalf("second replica did not see the page: %+v %v", got, ok)
	}
}
