package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/0xc0d3d00d/lighthouse/generated/mocks/httpserver/apimock"
	rbmodel "github.com/0xc0d3d00d/lighthouse/internal/recordbook/model"
	resolvermodel "github.com/0xc0d3d00d/lighthouse/internal/resolver/model"
)

func newTestServer(t *testing.T) (*http.Server, *apimock.MockRecordBook, *apimock.MockCache, *apimock.MockResolver) {
	t.Helper()
	book := apimock.NewMockRecordBook(t)
	cache := apimock.NewMockCache(t)
	resolver := apimock.NewMockResolver(t)
	srv := New(":0", apimock.NewMockHealth(t), apimock.NewMockStats(t), book, resolver, cache, nil, slog.New(slog.DiscardHandler))
	return srv, book, cache, resolver
}

func TestAddRecord(t *testing.T) {
	srv, book, cache, _ := newTestServer(t)

	var savedPacked []byte
	book.EXPECT().
		SaveManual("app.internal.", uint16(dns.TypeA), mock.Anything, 600*time.Second, 2).
		Run(func(_ string, _ uint16, packed []byte, _ time.Duration, _ int) { savedPacked = packed }).
		Return(nil)
	cache.EXPECT().Delete("app.internal.", uint16(dns.TypeA)).Return()
	book.EXPECT().Lookup("app.internal.", uint16(dns.TypeA)).RunAndReturn(
		func(name string, qtype uint16) (rbmodel.Record, error) {
			return rbmodel.Record{
				Name: name, Qtype: qtype, Packed: savedPacked, TTL: 600,
				QueriedAt: time.Now(), AnswerCount: 2, Manual: true,
			}, nil
		})

	body := `{"name":"app.internal","qtype":"a","ttl":600,"values":["10.0.0.5","10.0.0.6"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/records", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"manual":true`)
	assert.Contains(t, rec.Body.String(), "10.0.0.5")
	assert.Contains(t, rec.Body.String(), "10.0.0.6")

	// The stored bytes must unpack into a valid success response.
	msg := &dns.Msg{Data: savedPacked}
	require.NoError(t, msg.Unpack())
	assert.True(t, msg.Response)
	assert.Len(t, msg.Answer, 2)
}

func TestAddRecordValidation(t *testing.T) {
	for name, body := range map[string]string{
		"missing name":   `{"qtype":"A","values":["10.0.0.5"]}`,
		"unknown type":   `{"name":"a.example","qtype":"NOPE","values":["x"]}`,
		"no values":      `{"name":"a.example","qtype":"A","values":[]}`,
		"bad rdata":      `{"name":"a.example","qtype":"A","values":["not-an-ip"]}`,
		"malformed json": `{`,
	} {
		t.Run(name, func(t *testing.T) {
			srv, _, _, _ := newTestServer(t)
			req := httptest.NewRequest(http.MethodPost, "/api/records", strings.NewReader(body))
			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		})
	}
}

func TestRequeryManualConflict(t *testing.T) {
	srv, _, _, resolver := newTestServer(t)
	resolver.EXPECT().Requery(mock.Anything, "pin.example.", uint16(dns.TypeA)).
		Return(resolvermodel.Result{}, fmt.Errorf("pin.example./1: %w", rbmodel.ErrManualRecord))

	req := httptest.NewRequest(http.MethodPost, "/api/records/requery?name=pin.example&qtype=A", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
}
