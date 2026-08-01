package resolver_test

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/0xc0d3d00d/lighthouse/generated/mocks/resolvermock"
	rbmodel "github.com/0xc0d3d00d/lighthouse/internal/recordbook/model"
	"github.com/0xc0d3d00d/lighthouse/internal/resolver"
	"github.com/0xc0d3d00d/lighthouse/internal/resolver/backend"
	"github.com/0xc0d3d00d/lighthouse/internal/resolver/model"
)

// startBackend runs a local UDP DNS server. If empty is true it answers with
// no answer RRs, otherwise every query gets an A record 1.2.3.4.
func startBackend(t *testing.T, empty bool) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &dns.Server{
		PacketConn: pc,
		Handler: dns.HandlerFunc(func(_ context.Context, w dns.ResponseWriter, r *dns.Msg) {
			m := &dns.Msg{}
			m.ID = r.ID
			m.Response = true
			m.RecursionAvailable = true
			m.Question = r.Question
			if !empty && len(r.Question) > 0 {
				rr, err := dns.New(r.Question[0].Header().Name + " 300 IN A 1.2.3.4")
				if err == nil {
					m.Answer = []dns.RR{rr}
				}
			}
			_, _ = m.WriteTo(w)
		}),
	}
	go func() { _ = srv.ListenAndServe() }()
	// Closing the listener stops the server; dns.Server.Shutdown races with a
	// concurrently starting ListenAndServe, so it is deliberately avoided here.
	t.Cleanup(func() { _ = pc.Close() })
	return pc.LocalAddr().String()
}

type mocks struct {
	cache  *resolvermock.MockCache
	book   *resolvermock.MockRecordBook
	health *resolvermock.MockHealth
}

func newService(t *testing.T, addrs []string) (*resolver.Service, mocks) {
	t.Helper()
	m := mocks{
		cache:  resolvermock.NewMockCache(t),
		book:   resolvermock.NewMockRecordBook(t),
		health: resolvermock.NewMockHealth(t),
	}
	backends := make([]model.Backend, 0, len(addrs))
	for _, a := range addrs {
		backends = append(backends, model.Backend{Scheme: "udp", Address: a})
	}
	svc := resolver.New(backends, backend.NewClient(2*time.Second), m.cache, m.book, m.health, false, slog.New(slog.DiscardHandler))
	return svc, m
}

func newServiceBackends(t *testing.T, backends []model.Backend) (*resolver.Service, mocks) {
	t.Helper()
	m := mocks{
		cache:  resolvermock.NewMockCache(t),
		book:   resolvermock.NewMockRecordBook(t),
		health: resolvermock.NewMockHealth(t),
	}
	svc := resolver.New(backends, backend.NewClient(2*time.Second), m.cache, m.book, m.health, false, slog.New(slog.DiscardHandler))
	return svc, m
}

func TestResolveRoutesBySuffix(t *testing.T) {
	catchAllAddr := startBackend(t, true) // empty answers
	suffixAddr := startBackend(t, false)  // answers 1.2.3.4
	svc, m := newServiceBackends(t, []model.Backend{
		{Scheme: "udp", Address: catchAllAddr, Suffix: "."},
		{Scheme: "udp", Address: suffixAddr, Suffix: "cluster.local."},
	})

	// A cluster.local. query goes only to the suffix backend and gets an answer.
	m.cache.EXPECT().Get("foo.svc.cluster.local.", dns.TypeA).Return(nil, false)
	m.book.EXPECT().LookupManualPacked("foo.svc.cluster.local.", dns.TypeA).Return(nil, 0, false)
	m.health.EXPECT().Survival().Return(false)
	m.cache.EXPECT().Set("foo.svc.cluster.local.", dns.TypeA, mock.Anything, mock.Anything).Return()
	m.book.EXPECT().Save("foo.svc.cluster.local.", dns.TypeA, mock.Anything, mock.Anything, mock.Anything, 1, mock.Anything).Return(nil)

	res, err := svc.Resolve(context.Background(), "foo.svc.cluster.local.", dns.TypeA)
	require.NoError(t, err)
	assert.Equal(t, 1, res.AnswerCount)
	assert.Equal(t, "udp://"+suffixAddr, res.Source)

	// A non-matching query goes only to the catch-all backend: the suffix
	// backend would answer non-empty, so an empty result proves it was skipped.
	m.cache.EXPECT().Get("example.com.", dns.TypeA).Return(nil, false)
	m.book.EXPECT().LookupManualPacked("example.com.", dns.TypeA).Return(nil, 0, false)
	m.health.EXPECT().Survival().Return(false)

	res, err = svc.Resolve(context.Background(), "example.com.", dns.TypeA)
	require.NoError(t, err)
	assert.Equal(t, 0, res.AnswerCount)
}

func TestResolveNoMatchingBackend(t *testing.T) {
	addr := startBackend(t, false)
	svc, m := newServiceBackends(t, []model.Backend{
		{Scheme: "udp", Address: addr, Suffix: "cluster.local."},
	})

	m.cache.EXPECT().Get("example.com.", dns.TypeA).Return(nil, false)
	m.book.EXPECT().LookupManualPacked("example.com.", dns.TypeA).Return(nil, 0, false)
	m.health.EXPECT().Survival().Return(false)

	_, err := svc.Resolve(context.Background(), "example.com.", dns.TypeA)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no backend serves")
}

func TestResolveCacheHit(t *testing.T) {
	svc, m := newService(t, []string{"127.0.0.1:1"})
	m.cache.EXPECT().Get("example.com.", dns.TypeA).Return([]byte{0xaa, 0xbb}, true)

	res, err := svc.Resolve(context.Background(), "example.com.", dns.TypeA)
	require.NoError(t, err)
	assert.Equal(t, "cache", res.Source)
	assert.Equal(t, []byte{0xaa, 0xbb}, res.Packed)
}

func TestResolveFanOutStoresResult(t *testing.T) {
	addr := startBackend(t, false)
	svc, m := newService(t, []string{addr})

	m.cache.EXPECT().Get("example.com.", dns.TypeA).Return(nil, false)
	m.book.EXPECT().LookupManualPacked("example.com.", dns.TypeA).Return(nil, 0, false)
	m.health.EXPECT().Survival().Return(false)
	m.cache.EXPECT().Set("example.com.", dns.TypeA, mock.Anything, 300*time.Second).Return()
	m.book.EXPECT().Save("example.com.", dns.TypeA, mock.Anything, 300*time.Second, uint16(dns.RcodeSuccess), 1, mock.Anything).Return(nil)

	res, err := svc.Resolve(context.Background(), "example.com.", dns.TypeA)
	require.NoError(t, err)
	assert.Equal(t, 1, res.AnswerCount)
	assert.Equal(t, 300*time.Second, res.TTL)
	assert.NotEmpty(t, res.Packed)
}

func TestResolvePrefersNonEmpty(t *testing.T) {
	emptyAddr := startBackend(t, true)
	fullAddr := startBackend(t, false)
	svc, m := newService(t, []string{emptyAddr, fullAddr})

	m.cache.EXPECT().Get("example.com.", dns.TypeA).Return(nil, false)
	m.book.EXPECT().LookupManualPacked("example.com.", dns.TypeA).Return(nil, 0, false)
	m.health.EXPECT().Survival().Return(false)
	m.cache.EXPECT().Set("example.com.", dns.TypeA, mock.Anything, mock.Anything).Return()
	m.book.EXPECT().Save("example.com.", dns.TypeA, mock.Anything, mock.Anything, mock.Anything, 1, mock.Anything).Return(nil)

	res, err := svc.Resolve(context.Background(), "example.com.", dns.TypeA)
	require.NoError(t, err)
	assert.Equal(t, 1, res.AnswerCount, "expected the non-empty backend answer to win")
}

func TestResolveAllEmptyServesEmpty(t *testing.T) {
	addr := startBackend(t, true)
	svc, m := newService(t, []string{addr})

	m.cache.EXPECT().Get("example.com.", dns.TypeA).Return(nil, false)
	m.book.EXPECT().LookupManualPacked("example.com.", dns.TypeA).Return(nil, 0, false)
	m.health.EXPECT().Survival().Return(false)
	// No cache.Set / book.Save expectations: empty answers must not be stored
	// when StoreNegative is off; mockery fails the test on unexpected calls.

	res, err := svc.Resolve(context.Background(), "example.com.", dns.TypeA)
	require.NoError(t, err)
	assert.Equal(t, 0, res.AnswerCount)
}

func TestResolveSurvivalServesRecordBook(t *testing.T) {
	svc, m := newService(t, []string{"127.0.0.1:1"})

	m.cache.EXPECT().Get("example.com.", dns.TypeA).Return(nil, false)
	m.book.EXPECT().LookupManualPacked("example.com.", dns.TypeA).Return(nil, 0, false)
	m.health.EXPECT().Survival().Return(true)
	m.book.EXPECT().LookupPacked("example.com.", dns.TypeA).Return([]byte{0x01}, true)

	res, err := svc.Resolve(context.Background(), "example.com.", dns.TypeA)
	require.NoError(t, err)
	assert.Equal(t, "recordbook", res.Source)
	assert.True(t, res.Stale)
}

func TestResolveServesManualRecord(t *testing.T) {
	// No reachable backend: the manual record must be served without fan-out.
	svc, m := newService(t, []string{"127.0.0.1:1"})

	m.cache.EXPECT().Get("pin.example.", dns.TypeA).Return(nil, false)
	m.book.EXPECT().LookupManualPacked("pin.example.", dns.TypeA).Return([]byte{0x02}, 600*time.Second, true)
	m.cache.EXPECT().Set("pin.example.", dns.TypeA, []byte{0x02}, 600*time.Second).Return()

	res, err := svc.Resolve(context.Background(), "pin.example.", dns.TypeA)
	require.NoError(t, err)
	assert.Equal(t, "manual", res.Source)
	assert.Equal(t, []byte{0x02}, res.Packed)
}

func TestRequeryRejectsManualRecord(t *testing.T) {
	svc, m := newService(t, []string{"127.0.0.1:1"})

	m.health.EXPECT().Survival().Return(false)
	m.book.EXPECT().LookupManualPacked("pin.example.", dns.TypeA).Return([]byte{0x02}, 600*time.Second, true)

	_, err := svc.Requery(context.Background(), "pin.example.", dns.TypeA)
	assert.ErrorIs(t, err, rbmodel.ErrManualRecord)
}

func TestResolveSurvivalMissErrors(t *testing.T) {
	svc, m := newService(t, []string{"127.0.0.1:1"})

	m.cache.EXPECT().Get("missing.example.", dns.TypeA).Return(nil, false)
	m.book.EXPECT().LookupManualPacked("missing.example.", dns.TypeA).Return(nil, 0, false)
	m.health.EXPECT().Survival().Return(true)
	m.book.EXPECT().LookupPacked("missing.example.", dns.TypeA).Return(nil, false)

	_, err := svc.Resolve(context.Background(), "missing.example.", dns.TypeA)
	assert.Error(t, err)
}
