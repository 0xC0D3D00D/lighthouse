// Package api serves the dashboard SPA and its JSON API.
package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"codeberg.org/miekg/dns"

	healthmodel "github.com/0xc0d3d00d/lighthouse/internal/health/model"
	rbmodel "github.com/0xc0d3d00d/lighthouse/internal/recordbook/model"
	resolvermodel "github.com/0xc0d3d00d/lighthouse/internal/resolver/model"
	statsmodel "github.com/0xc0d3d00d/lighthouse/internal/stats/model"
)

type Health interface {
	Status() healthmodel.Status
}

type Stats interface {
	Snapshot(seconds int) statsmodel.Snapshot
}

type RecordBook interface {
	List(prefix string, qtype uint16, offset, limit int) ([]rbmodel.Record, int, error)
	Lookup(name string, qtype uint16) (rbmodel.Record, error)
	Delete(name string, qtype uint16) error
	Count() int64
}

type Resolver interface {
	Requery(ctx context.Context, name string, qtype uint16) (resolvermodel.Result, error)
	Backends() []resolvermodel.Backend
}

type Cache interface {
	Len() int
	Delete(name string, qtype uint16)
}

type Server struct {
	health   Health
	stats    Stats
	book     RecordBook
	resolver Resolver
	cache    Cache
	log      *slog.Logger
}

// New builds the dashboard HTTP server. webFS is the embedded SPA (rooted at
// its index.html directory); pass nil to serve the API only.
func New(addr string, h Health, st Stats, book RecordBook, r Resolver, c Cache, webFS fs.FS, log *slog.Logger) *http.Server {
	s := &Server{health: h, stats: st, book: book, resolver: r, cache: c, log: log.With("component", "api")}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/records", s.handleListRecords)
	mux.HandleFunc("DELETE /api/records", s.handleDeleteRecord)
	mux.HandleFunc("POST /api/records/requery", s.handleRequery)

	if webFS != nil {
		fileServer := http.FileServer(http.FS(webFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
			path := strings.TrimPrefix(req.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}
			if _, err := fs.Stat(webFS, path); err != nil {
				// SPA fallback: unknown paths get index.html so client routing works.
				req.URL.Path = "/"
			}
			fileServer.ServeHTTP(w, req)
		})
	}

	return &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	st := s.health.Status()
	ups := make([]string, 0)
	for _, u := range s.resolver.Backends() {
		ups = append(ups, u.Name())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"health":            st,
		"backends":         ups,
		"cache_entries":     s.cache.Len(),
		"recordbook_records": s.book.Count(),
	})
}

func (s *Server) handleStats(w http.ResponseWriter, req *http.Request) {
	seconds, _ := strconv.Atoi(req.URL.Query().Get("seconds"))
	if seconds <= 0 {
		seconds = 300
	}
	writeJSON(w, http.StatusOK, s.stats.Snapshot(seconds))
}

// recordView is a record enriched with human-readable answers for the explorer.
type recordView struct {
	rbmodel.Record
	Answers []string `json:"answers"`
	Expired bool     `json:"expired"`
}

func toView(rec rbmodel.Record) recordView {
	v := recordView{Record: rec}
	v.Packed = nil // wire bytes are not useful in the UI
	msg := &dns.Msg{Data: rec.Packed}
	if err := msg.Unpack(); err == nil {
		for _, rr := range msg.Answer {
			v.Answers = append(v.Answers, rr.String())
		}
	}
	v.Expired = time.Since(rec.QueriedAt) > time.Duration(rec.TTL)*time.Second
	return v
}

func (s *Server) handleListRecords(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	prefix := q.Get("search")
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	var qtype uint16
	if t := q.Get("qtype"); t != "" {
		qtype = dns.StringToType[strings.ToUpper(t)]
	}

	recs, total, err := s.book.List(prefix, qtype, offset, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]recordView, 0, len(recs))
	for _, rec := range recs {
		views = append(views, toView(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": views, "total": total})
}

func recordKeyParams(req *http.Request) (string, uint16, bool) {
	q := req.URL.Query()
	name := q.Get("name")
	if name != "" && !strings.HasSuffix(name, ".") {
		name += "."
	}
	qtype := dns.StringToType[strings.ToUpper(q.Get("qtype"))]
	return name, qtype, name != "" && qtype != 0
}

func (s *Server) handleDeleteRecord(w http.ResponseWriter, req *http.Request) {
	name, qtype, ok := recordKeyParams(req)
	if !ok {
		writeErr(w, http.StatusBadRequest, "name and qtype query params are required")
		return
	}
	if err := s.book.Delete(name, qtype); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	s.cache.Delete(name, qtype)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleRequery(w http.ResponseWriter, req *http.Request) {
	name, qtype, ok := recordKeyParams(req)
	if !ok {
		writeErr(w, http.StatusBadRequest, "name and qtype query params are required")
		return
	}
	res, err := s.resolver.Requery(req.Context(), name, qtype)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	rec, err := s.book.Lookup(name, qtype)
	if err != nil {
		// Negative answers may not be persisted; synthesize a view from the result.
		rec = rbmodel.Record{
			Name: name, Qtype: qtype, Packed: res.Packed,
			TTL: uint32(res.TTL / time.Second), QueriedAt: time.Now(),
			Rcode: res.Rcode, AnswerCount: res.AnswerCount,
		}
	}
	writeJSON(w, http.StatusOK, toView(rec))
}
