package health

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/0xc0d3d00d/lighthouse/generated/mocks/healthmock"

	"github.com/0xc0d3d00d/lighthouse/internal/health/model"
)

// newProber returns a mock Prober whose per-backend beacon reachability
// follows the ok flag.
func newProber(t *testing.T) (*healthmock.MockProber, *atomic.Bool) {
	t.Helper()
	ok := &atomic.Bool{}
	p := healthmock.NewMockProber(t)
	p.EXPECT().Probe(mock.Anything, "beacon.example.", uint16(1)).RunAndReturn(
		func(context.Context, string, uint16) map[string]bool {
			return map[string]bool{"udp://1.1.1.1:53": ok.Load(), "udp://8.8.8.8:53": ok.Load()}
		}).Maybe()
	return p, ok
}

func newTestService(p Prober) *Service {
	return New(p, Options{
		BeaconName:   "beacon.example.",
		BeaconType:   1,
		Interval:     time.Hour, // probes driven manually in tests
		FailAfter:    30 * time.Millisecond,
		RecoverAfter: 30 * time.Millisecond,
	}, slog.New(slog.DiscardHandler))
}

func TestSurvivalTransitions(t *testing.T) {
	p, ok := newProber(t)
	s := newTestService(p)
	ctx := context.Background()

	// Beacon down but FailAfter not yet elapsed: stay normal.
	s.probe(ctx)
	require.False(t, s.Survival(), "entered survival before FailAfter elapsed")

	// Beacon down past FailAfter: survival.
	time.Sleep(40 * time.Millisecond)
	s.probe(ctx)
	require.True(t, s.Survival(), "expected survival mode after FailAfter of beacon failures")

	// Beacon back, but RecoverAfter not yet elapsed: stay in survival.
	ok.Store(true)
	s.probe(ctx)
	require.True(t, s.Survival(), "left survival before RecoverAfter elapsed")

	// Beacon continuously up past RecoverAfter: back to normal.
	time.Sleep(40 * time.Millisecond)
	s.probe(ctx)
	assert.False(t, s.Survival(), "expected normal mode after RecoverAfter of beacon success")
}

func TestRecoveryStreakResetsOnFailure(t *testing.T) {
	p, ok := newProber(t)
	s := newTestService(p)
	ctx := context.Background()

	time.Sleep(40 * time.Millisecond)
	s.probe(ctx)
	require.True(t, s.Survival(), "expected survival mode")

	// Success then failure: the streak resets, so survival persists even
	// after RecoverAfter from the first success.
	ok.Store(true)
	s.probe(ctx)
	ok.Store(false)
	s.probe(ctx)
	time.Sleep(40 * time.Millisecond)
	ok.Store(true)
	s.probe(ctx)
	assert.True(t, s.Survival(), "streak reset ignored: left survival too early")
}

func TestStatusReportsBackends(t *testing.T) {
	p, ok := newProber(t)
	ok.Store(true)
	s := newTestService(p)
	s.probe(context.Background())

	st := s.Status()
	assert.Equal(t, model.ModeNormal, st.Mode)
	assert.Len(t, st.Backends, 2)
}
