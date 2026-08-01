package health

import (
	"context"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/0xc0d3d00d/lighthouse/internal/health/model"
	"github.com/0xc0d3d00d/lighthouse/internal/metrics"
)

// Prober queries the beacon record on every backend (implemented by an adapter
// over the resolver service).
type Prober interface {
	Probe(ctx context.Context, name string, qtype uint16) map[string]bool
}

type Options struct {
	BeaconName   string
	BeaconType   uint16
	Interval     time.Duration
	FailAfter    time.Duration
	RecoverAfter time.Duration
}

type Service struct {
	prober Prober
	opts   Options
	log    *slog.Logger

	mu            sync.RWMutex
	mode          model.Mode
	modeSince     time.Time
	lastBeaconOK  time.Time
	beaconOKSince time.Time // start of the current uninterrupted success streak
	backends     map[string]model.BackendStatus
}

func New(prober Prober, opts Options, log *slog.Logger) *Service {
	now := time.Now()
	return &Service{
		prober:        prober,
		opts:          opts,
		log:           log.With("service", "health"),
		mode:          model.ModeNormal,
		modeSince:     now,
		lastBeaconOK:  now, // grace period: assume reachable at startup
		backends:     make(map[string]model.BackendStatus),
	}
}

// Run probes the beacon on an interval until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.opts.Interval)
	defer ticker.Stop()
	s.probe(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.probe(ctx)
		}
	}
}

func (s *Service) probe(ctx context.Context) {
	results := s.prober.Probe(ctx, s.opts.BeaconName, s.opts.BeaconType)
	now := time.Now()

	anyOK := false
	for _, ok := range results {
		if ok {
			anyOK = true
			break
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for name, ok := range results {
		s.backends[name] = model.BackendStatus{Name: name, Reachable: ok, CheckedAt: now}
	}

	if anyOK {
		s.lastBeaconOK = now
		if s.beaconOKSince.IsZero() {
			s.beaconOKSince = now
		}
	} else {
		s.beaconOKSince = time.Time{}
	}

	switch s.mode {
	case model.ModeNormal:
		if now.Sub(s.lastBeaconOK) >= s.opts.FailAfter {
			s.mode = model.ModeSurvival
			s.modeSince = now
			metrics.SurvivalMode.Set(1)
			s.log.Warn("beacon unreachable on all backends, entering survival mode",
				"beacon", s.opts.BeaconName, "fail_after", s.opts.FailAfter)
		}
	case model.ModeSurvival:
		if !s.beaconOKSince.IsZero() && now.Sub(s.beaconOKSince) >= s.opts.RecoverAfter {
			s.mode = model.ModeNormal
			s.modeSince = now
			metrics.SurvivalMode.Set(0)
			s.log.Info("beacon reachable again, back to normal mode",
				"beacon", s.opts.BeaconName, "recover_after", s.opts.RecoverAfter)
		}
	}
}

func (s *Service) Survival() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode == model.ModeSurvival
}

func (s *Service) Status() model.Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ups := make([]model.BackendStatus, 0, len(s.backends))
	for _, u := range s.backends {
		ups = append(ups, u)
	}
	sort.Slice(ups, func(i, j int) bool { return ups[i].Name < ups[j].Name })
	return model.Status{
		Mode:          s.mode,
		Backends:     ups,
		LastBeaconOK:  s.lastBeaconOK,
		BeaconOKSince: s.beaconOKSince,
		ModeSince:     s.modeSince,
	}
}
