package cache

import (
	"fmt"
	"time"

	"github.com/maypok86/otter/v2"

	"github.com/0xc0d3d00d/lighthouse/internal/cache/model"
)

type Service struct {
	cache *otter.Cache[string, model.Entry]
}

func New(maxSize int) (*Service, error) {
	c, err := otter.New(&otter.Options[string, model.Entry]{
		MaximumSize: maxSize,
		ExpiryCalculator: otter.ExpiryCreatingFunc(func(e otter.Entry[string, model.Entry]) time.Duration {
			return e.Value.TTL
		}),
	})
	if err != nil {
		return nil, err
	}
	return &Service{cache: c}, nil
}

func key(name string, qtype uint16) string {
	return fmt.Sprintf("%s|%d", name, qtype)
}

// Get returns the packed DNS response cached for (name, qtype).
func (s *Service) Get(name string, qtype uint16) ([]byte, bool) {
	e, ok := s.cache.GetIfPresent(key(name, qtype))
	if !ok {
		return nil, false
	}
	return e.Packed, true
}

// Set caches a packed DNS response for the record's remaining TTL.
func (s *Service) Set(name string, qtype uint16, packed []byte, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	s.cache.Set(key(name, qtype), model.Entry{
		Packed:   packed,
		TTL:      ttl,
		StoredAt: time.Now(),
	})
}

func (s *Service) Delete(name string, qtype uint16) {
	s.cache.Invalidate(key(name, qtype))
}

func (s *Service) Len() int {
	return s.cache.EstimatedSize()
}

func (s *Service) Close() {
	s.cache.StopAllGoroutines()
}
