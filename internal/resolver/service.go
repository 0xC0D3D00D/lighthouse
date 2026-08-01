package resolver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"codeberg.org/miekg/dns"

	"github.com/0xc0d3d00d/lighthouse/internal/metrics"
	rbmodel "github.com/0xc0d3d00d/lighthouse/internal/recordbook/model"
	"github.com/0xc0d3d00d/lighthouse/internal/resolver/model"
	"github.com/0xc0d3d00d/lighthouse/internal/resolver/backend"
)

// Cache is the resolver's view of the in-memory cache (implemented by an adapter).
type Cache interface {
	Get(name string, qtype uint16) (packed []byte, ok bool)
	Set(name string, qtype uint16, packed []byte, ttl time.Duration)
}

// RecordBook is the resolver's view of the persistent record book.
type RecordBook interface {
	Save(name string, qtype uint16, packed []byte, ttl time.Duration, rcode uint16, answers int, source string) error
	LookupPacked(name string, qtype uint16) (packed []byte, ok bool)
	LookupManualPacked(name string, qtype uint16) (packed []byte, ttl time.Duration, ok bool)
}

// Health is the resolver's view of the beacon health service (implemented by an adapter).
type Health interface {
	Survival() bool
}

type Service struct {
	backends  []model.Backend
	client     *backend.Client
	cache      Cache
	recordBook RecordBook
	health     Health
	storeNeg   bool
	log        *slog.Logger
}

func New(ups []model.Backend, client *backend.Client, cache Cache, book RecordBook, health Health, storeNegative bool, log *slog.Logger) *Service {
	return &Service{
		backends:  ups,
		client:     client,
		cache:      cache,
		recordBook: book,
		health:     health,
		storeNeg:   storeNegative,
		log:        log.With("service", "resolver"),
	}
}

func (s *Service) Backends() []model.Backend {
	return s.backends
}

// Resolve answers a single question: cache first, then backend fan-out
// (fastest non-empty wins) in normal mode, or the record book in survival mode.
func (s *Service) Resolve(ctx context.Context, name string, qtype uint16) (model.Result, error) {
	if packed, ok := s.cache.Get(name, qtype); ok {
		metrics.CacheHits.Inc()
		return model.Result{Packed: packed, Source: "cache"}, nil
	}

	// Manually added records are the source of truth: serve them without
	// consulting backends, in normal and survival mode alike.
	if packed, ttl, ok := s.recordBook.LookupManualPacked(name, qtype); ok {
		s.cache.Set(name, qtype, packed, ttl)
		return model.Result{Packed: packed, Source: "manual"}, nil
	}

	if s.health.Survival() {
		if packed, ok := s.recordBook.LookupPacked(name, qtype); ok {
			return model.Result{Packed: packed, Source: "recordbook", Stale: true}, nil
		}
		return model.Result{}, fmt.Errorf("survival mode: %s/%d not in record book", name, qtype)
	}

	res, err := s.fanOut(ctx, name, qtype)
	if err != nil {
		return model.Result{}, err
	}

	if res.AnswerCount > 0 && res.Rcode == dns.RcodeSuccess {
		s.cache.Set(name, qtype, res.Packed, res.TTL)
	}
	if res.AnswerCount > 0 || s.storeNeg {
		if err := s.recordBook.Save(name, qtype, res.Packed, res.TTL, res.Rcode, res.AnswerCount, res.Source); err != nil && !errors.Is(err, rbmodel.ErrManualRecord) {
			s.log.Warn("record book save failed", "name", name, "qtype", qtype, "err", err)
		}
	}
	return res, nil
}

// Requery bypasses the cache and record book and queries backends directly,
// refreshing both stores on success. Used by the dashboard's live re-query.
// Manually added records cannot be re-queried; delete them first.
func (s *Service) Requery(ctx context.Context, name string, qtype uint16) (model.Result, error) {
	if s.health.Survival() {
		return model.Result{}, fmt.Errorf("survival mode: backend queries are disabled")
	}
	if _, _, ok := s.recordBook.LookupManualPacked(name, qtype); ok {
		return model.Result{}, fmt.Errorf("%s/%d: %w", name, qtype, rbmodel.ErrManualRecord)
	}
	res, err := s.fanOut(ctx, name, qtype)
	if err != nil {
		return model.Result{}, err
	}
	if res.AnswerCount > 0 && res.Rcode == dns.RcodeSuccess {
		s.cache.Set(name, qtype, res.Packed, res.TTL)
	}
	if res.AnswerCount > 0 || s.storeNeg {
		if err := s.recordBook.Save(name, qtype, res.Packed, res.TTL, res.Rcode, res.AnswerCount, res.Source); err != nil {
			if errors.Is(err, rbmodel.ErrManualRecord) {
				return model.Result{}, fmt.Errorf("%s/%d: %w", name, qtype, rbmodel.ErrManualRecord)
			}
			s.log.Warn("record book save failed", "name", name, "qtype", qtype, "err", err)
		}
	}
	return res, nil
}

// backendsFor selects the backends whose suffix routes the given name: among
// the suffixes that match, only the most specific one (most labels) wins, and
// every backend configured with that winning suffix is returned. The catch-all
// "." suffix is the least specific.
func (s *Service) backendsFor(name string) []model.Backend {
	best := -1
	var out []model.Backend
	for _, up := range s.backends {
		if !up.Matches(name) {
			continue
		}
		spec := suffixSpecificity(up.Suffix)
		switch {
		case spec > best:
			best = spec
			out = out[:0]
			out = append(out, up)
		case spec == best:
			out = append(out, up)
		}
	}
	return out
}

// suffixSpecificity is the number of labels in a canonical suffix; "." is 0.
func suffixSpecificity(suffix string) int {
	if suffix == "" || suffix == "." {
		return 0
	}
	return strings.Count(suffix, ".")
}

// fanOut queries all backends concurrently and returns the fastest non-empty
// response; if every backend answers empty, the first successful (empty)
// response is returned; if all fail, an error is returned.
func (s *Service) fanOut(ctx context.Context, name string, qtype uint16) (model.Result, error) {
	type answer struct {
		res model.Result
		err error
	}
	backends := s.backendsFor(name)
	if len(backends) == 0 {
		return model.Result{}, fmt.Errorf("no backend serves %q", name)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan answer, len(backends))
	for _, up := range backends {
		go func(up model.Backend) {
			// A panic in an unrecovered goroutine kills the process; report it
			// as a backend failure instead.
			defer func() {
				if rec := recover(); rec != nil {
					s.log.Error("panic querying backend", "backend", up.Name(), "panic", rec, "stack", string(debug.Stack()))
					ch <- answer{err: fmt.Errorf("%s: panic: %v", up.Name(), rec)}
				}
			}()
			metrics.BackendQueries.WithLabelValues(up.Name()).Inc()
			resp, err := s.client.Query(ctx, up, name, qtype)
			if err != nil {
				metrics.BackendErrors.WithLabelValues(up.Name()).Inc()
				ch <- answer{err: fmt.Errorf("%s: %w", up.Name(), err)}
				return
			}
			ch <- answer{res: toResult(resp, up.Name())}
		}(up)
	}

	var firstEmpty *model.Result
	var lastErr error
	for range backends {
		a := <-ch
		if a.err != nil {
			lastErr = a.err
			continue
		}
		if a.res.AnswerCount > 0 && a.res.Rcode == dns.RcodeSuccess {
			return a.res, nil
		}
		if firstEmpty == nil {
			r := a.res
			firstEmpty = &r
		}
	}
	if firstEmpty != nil {
		return *firstEmpty, nil
	}
	return model.Result{}, fmt.Errorf("all backends failed: %w", lastErr)
}

// Probe queries name/qtype on every backend and reports reachability per backend.
func (s *Service) Probe(ctx context.Context, name string, qtype uint16) map[string]bool {
	type probe struct {
		name string
		ok   bool
	}
	ch := make(chan probe, len(s.backends))
	for _, up := range s.backends {
		go func(up model.Backend) {
			defer func() {
				if rec := recover(); rec != nil {
					s.log.Error("panic probing backend", "backend", up.Name(), "panic", rec, "stack", string(debug.Stack()))
					ch <- probe{name: up.Name(), ok: false}
				}
			}()
			resp, err := s.client.Query(ctx, up, name, qtype)
			ch <- probe{name: up.Name(), ok: err == nil && resp.Rcode == dns.RcodeSuccess && len(resp.Answer) > 0}
		}(up)
	}
	out := make(map[string]bool, len(s.backends))
	for range s.backends {
		p := <-ch
		out[p.name] = p.ok
	}
	return out
}

func toResult(resp *dns.Msg, source string) model.Result {
	minTTL := time.Duration(0)
	for _, rr := range resp.Answer {
		ttl := time.Duration(rr.Header().TTL) * time.Second
		if minTTL == 0 || ttl < minTTL {
			minTTL = ttl
		}
	}
	return model.Result{
		Packed:      resp.Data,
		Rcode:       resp.Rcode,
		AnswerCount: len(resp.Answer),
		TTL:         minTTL,
		Source:      source,
	}
}
