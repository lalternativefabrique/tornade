package crawl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	jobsBucket  = "vvaves_crawl_jobs"
	pagesBucket = "vvaves_crawl_pages"
	streamName  = "VVAVES_CRAWL"
	subject     = "vvaves.crawl.jobs"
	consumer    = "crawler"

	// jobTTL is how long a finished crawl stays readable. A caller polls
	// within minutes; a day covers the one that comes back tomorrow.
	jobTTL = 24 * time.Hour
	// ackWait bounds one attempt at a job before the queue hands it to
	// another replica; the runner keeps the attempt alive while it works.
	ackWait   = 5 * time.Minute
	opTimeout = 5 * time.Second
)

// NATSStore keeps jobs and pages in two JetStream KV buckets, so the replica
// answering a status request need not be the one running the crawl.
type NATSStore struct {
	jobs  jetstream.KeyValue
	pages jetstream.KeyValue
}

func NewNATSStore(nc *nats.Conn, maxBytes int64) (*NATSStore, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	jobs, err := openBucket(ctx, js, jobsBucket, maxBytes/8)
	if err != nil {
		return nil, err
	}
	pages, err := openBucket(ctx, js, pagesBucket, maxBytes)
	if err != nil {
		return nil, err
	}
	return &NATSStore{jobs: jobs, pages: pages}, nil
}

func openBucket(ctx context.Context, js jetstream.JetStream, name string, maxBytes int64) (jetstream.KeyValue, error) {
	kv, err := js.KeyValue(ctx, name)
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		kv, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket: name, TTL: jobTTL, MaxBytes: maxBytes, Storage: jetstream.FileStorage,
		})
	}
	return kv, err
}

func (s *NATSStore) Put(ctx context.Context, job Job) error {
	value, err := json.Marshal(job)
	if err != nil {
		return err
	}
	_, err = s.jobs.Put(ctx, job.ID, value)
	return err
}

func (s *NATSStore) Get(ctx context.Context, id string) (Job, error) {
	entry, err := s.jobs.Get(ctx, id)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	var job Job
	if err := json.Unmarshal(entry.Value(), &job); err != nil {
		return Job{}, err
	}
	return job, nil
}

func pageKey(id string, seq int) string {
	return fmt.Sprintf("%s.%06d", id, seq)
}

func (s *NATSStore) AddPage(ctx context.Context, id string, seq int, page StoredPage) error {
	value, err := json.Marshal(page)
	if err != nil {
		return err
	}
	_, err = s.pages.Put(ctx, pageKey(id, seq), value)
	return err
}

// Pages reads the sequence one key at a time: a job's pages are numbered
// without gaps, so the first missing key is the end.
func (s *NATSStore) Pages(ctx context.Context, id string, offset, limit int) ([]StoredPage, error) {
	if limit <= 0 {
		limit = MaxMaxPages
	}
	var out []StoredPage
	for seq := offset; seq < offset+limit; seq++ {
		entry, err := s.pages.Get(ctx, pageKey(id, seq))
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			break
		}
		if err != nil {
			return nil, err
		}
		var page StoredPage
		if err := json.Unmarshal(entry.Value(), &page); err != nil {
			return nil, err
		}
		out = append(out, page)
	}
	return out, nil
}

// NATSQueue is a JetStream work queue: a job is delivered to one replica,
// and comes back to another if that one dies before acknowledging it.
type NATSQueue struct {
	js jetstream.JetStream
}

func NewNATSQueue(nc *nats.Conn) (*NATSQueue, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: streamName, Subjects: []string{subject}, Retention: jetstream.WorkQueuePolicy, Storage: jetstream.FileStorage,
	})
	if err != nil {
		return nil, err
	}
	return &NATSQueue{js: js}, nil
}

func (q *NATSQueue) Enqueue(ctx context.Context, id string) error {
	_, err := q.js.Publish(ctx, subject, []byte(id))
	return err
}

func (q *NATSQueue) Consume(ctx context.Context, handle func(context.Context, string) error) error {
	cons, err := q.js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		Durable: consumer, AckPolicy: jetstream.AckExplicitPolicy, AckWait: ackWait, MaxDeliver: 3, FilterSubject: subject,
	})
	if err != nil {
		return err
	}
	for ctx.Err() == nil {
		msgs, err := cons.Fetch(1, jetstream.FetchMaxWait(10*time.Second))
		if err != nil {
			continue
		}
		for msg := range msgs.Messages() {
			q.work(ctx, msg, handle)
		}
	}
	return ctx.Err()
}

// work runs one job while telling the queue it is still alive, so a crawl
// longer than ackWait is not handed to a second replica mid-way.
func (q *NATSQueue) work(ctx context.Context, msg jetstream.Msg, handle func(context.Context, string) error) {
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		t := time.NewTicker(ackWait / 3)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				msg.InProgress()
			case <-jobCtx.Done():
				return
			}
		}
	}()
	if err := handle(jobCtx, string(msg.Data())); err != nil {
		msg.Nak()
		return
	}
	msg.Ack()
}
