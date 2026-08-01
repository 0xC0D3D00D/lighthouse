package stats

import (
	"sync"
	"time"

	"github.com/0xc0d3d00d/lighthouse/internal/stats/model"
)

// Service keeps a rolling window of per-second counters in a ring buffer.
type Service struct {
	mu      sync.Mutex
	ring    []model.Bucket
	size    int
	totals  model.Snapshot
}

// New creates a stats service holding `window` seconds of history.
func New(window time.Duration) *Service {
	size := int(window / time.Second)
	if size < 60 {
		size = 60
	}
	return &Service{ring: make([]model.Bucket, size), size: size}
}

func (s *Service) bucket(now time.Time) *model.Bucket {
	unix := now.Unix()
	b := &s.ring[unix%int64(s.size)]
	if b.Unix != unix {
		*b = model.Bucket{Unix: unix}
	}
	return b
}

func (s *Service) RecordQuery(cacheHit bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.bucket(time.Now())
	b.Queries++
	s.totals.TotalQueries++
	if cacheHit {
		b.CacheHits++
		s.totals.TotalCacheHits++
	}
}

func (s *Service) RecordError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.bucket(time.Now())
	b.Errors++
	s.totals.TotalErrors++
}

func (s *Service) RecordBackendError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bucket(time.Now()).BackendErrors++
}

// Snapshot returns the last `seconds` seconds of buckets, oldest first.
func (s *Service) Snapshot(seconds int) model.Snapshot {
	if seconds <= 0 || seconds > s.size {
		seconds = s.size
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().Unix()
	out := make([]model.Bucket, 0, seconds)
	for i := now - int64(seconds) + 1; i <= now; i++ {
		b := s.ring[i%int64(s.size)]
		if b.Unix != i {
			b = model.Bucket{Unix: i}
		}
		out = append(out, b)
	}
	snap := s.totals
	snap.Buckets = out
	return snap
}
