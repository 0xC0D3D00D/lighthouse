package dnsserver

import (
	"context"
	"log/slog"
	"testing"

	"codeberg.org/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0xc0d3d00d/lighthouse/generated/mocks/dnsservermock"
)

// TestAnswerRecoversPanic pins the recover guard: the dns library does not
// recover handler panics, so a panicking resolve path must be turned into a
// SERVFAIL response instead of crashing the process.
func TestAnswerRecoversPanic(t *testing.T) {
	resolver := dnsservermock.NewMockResolver(t)
	stats := dnsservermock.NewMockStats(t)
	resolver.EXPECT().Resolve(context.Background(), "example.com.", dns.TypeA).Panic("boom")
	stats.EXPECT().RecordError().Return()

	s := New(resolver, stats, Options{}, slog.New(slog.DiscardHandler))

	q := dns.NewMsg("example.com.", dns.TypeA)
	require.NotNil(t, q)
	q.ID = 0x1234

	out := s.answer(context.Background(), "udp", q)
	require.NotNil(t, out, "panic must yield a SERVFAIL response, not nil")

	resp := &dns.Msg{Data: out}
	require.NoError(t, resp.Unpack())
	assert.Equal(t, uint16(dns.RcodeServerFailure), resp.Rcode)
	assert.Equal(t, uint16(0x1234), resp.ID)
}
