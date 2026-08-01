package dnsserver

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"runtime/debug"
	"log/slog"
	"net/http"
	"time"

	"codeberg.org/miekg/dns"

	"github.com/0xc0d3d00d/lighthouse/internal/metrics"
	"github.com/0xc0d3d00d/lighthouse/internal/resolver/model"
)

// Resolver is the consumer interface the DNS edge needs from the resolver service.
type Resolver interface {
	Resolve(ctx context.Context, name string, qtype uint16) (model.Result, error)
}

// Stats is the consumer interface for the stats ring buffer.
type Stats interface {
	RecordQuery(cacheHit bool)
	RecordError()
}

type Options struct {
	UDPAddr string
	TCPAddr string
	DoTAddr string
	DoHAddr string
	TLSCert string
	TLSKey  string
}

type Server struct {
	resolver Resolver
	stats    Stats
	opts     Options
	log      *slog.Logger

	servers []*dns.Server
	doh     *http.Server
}

func New(r Resolver, st Stats, opts Options, log *slog.Logger) *Server {
	return &Server{resolver: r, stats: st, opts: opts, log: log.With("component", "dnsserver")}
}

// Start launches all configured listeners. Errors are reported on errCh.
func (s *Server) Start(errCh chan<- error) error {
	var tlsConf *tls.Config
	if s.opts.DoTAddr != "" || s.opts.DoHAddr != "" {
		cert, err := tls.LoadX509KeyPair(s.opts.TLSCert, s.opts.TLSKey)
		if err != nil {
			return fmt.Errorf("load TLS keypair: %w", err)
		}
		tlsConf = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	}

	start := func(srv *dns.Server, proto string) {
		s.servers = append(s.servers, srv)
		go func() {
			s.log.Info("dns listener starting", "proto", proto, "addr", srv.Addr)
			if err := srv.ListenAndServe(); err != nil {
				errCh <- fmt.Errorf("%s listener: %w", proto, err)
			}
		}()
	}

	if s.opts.UDPAddr != "" {
		start(&dns.Server{Addr: s.opts.UDPAddr, Net: "udp", Handler: s.handler("udp")}, "udp")
	}
	if s.opts.TCPAddr != "" {
		start(&dns.Server{Addr: s.opts.TCPAddr, Net: "tcp", Handler: s.handler("tcp")}, "tcp")
	}
	if s.opts.DoTAddr != "" {
		dot := tlsConf.Clone()
		dot.NextProtos = []string{"dot"}
		start(&dns.Server{Addr: s.opts.DoTAddr, Net: "tcp", TLSConfig: dot, Handler: s.handler("dot")}, "dot")
	}
	if s.opts.DoHAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/dns-query", s.serveDoH)
		doh := tlsConf.Clone()
		doh.NextProtos = []string{"h2", "http/1.1"}
		s.doh = &http.Server{
			Addr:              s.opts.DoHAddr,
			Handler:           mux,
			TLSConfig:         doh,
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			s.log.Info("dns listener starting", "proto", "doh", "addr", s.opts.DoHAddr)
			if err := s.doh.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("doh listener: %w", err)
			}
		}()
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, srv := range s.servers {
			srv.Shutdown(ctx)
		}
		if s.doh != nil {
			_ = s.doh.Shutdown(ctx)
		}
	}()
	// dns.Server.Shutdown can block forever on a listener that never started
	// (e.g. its bind failed); don't let that wedge process exit.
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (s *Server) handler(proto string) dns.Handler {
	return dns.HandlerFunc(s.serveDNS(proto))
}

// answer resolves the request and returns the packed response bytes.
// A panic anywhere in the resolve path is answered with SERVFAIL instead of
// crashing the process: the dns library does not recover handler panics.
func (s *Server) answer(ctx context.Context, proto string, r *dns.Msg) (out []byte) {
	defer func() {
		if rec := recover(); rec != nil {
			s.stats.RecordError()
			metrics.FrontendErrors.WithLabelValues(proto).Inc()
			s.log.Error("panic while answering query", "proto", proto, "panic", rec, "stack", string(debug.Stack()))
			out = packedError(r, dns.RcodeServerFailure)
		}
	}()
	metrics.FrontendQueries.WithLabelValues(proto).Inc()

	if len(r.Question) == 0 {
		s.stats.RecordError()
		metrics.FrontendErrors.WithLabelValues(proto).Inc()
		return packedError(r, dns.RcodeFormatError)
	}
	q := r.Question[0]
	name := q.Header().Name
	qtype := dns.RRToType(q)

	res, err := s.resolver.Resolve(ctx, name, qtype)
	if err != nil {
		s.stats.RecordError()
		metrics.FrontendErrors.WithLabelValues(proto).Inc()
		s.log.Debug("resolve failed", "name", name, "qtype", qtype, "err", err)
		return packedError(r, dns.RcodeServerFailure)
	}
	s.stats.RecordQuery(res.Source == "cache")

	// Rewrite the stored/backend message ID with the client's ID.
	out = make([]byte, len(res.Packed))
	copy(out, res.Packed)
	if len(out) >= 2 {
		out[0] = byte(r.ID >> 8)
		out[1] = byte(r.ID)
	}
	return out
}

// packedError builds a minimal error response for the request.
func packedError(r *dns.Msg, rcode uint16) []byte {
	m := &dns.Msg{}
	m.ID = r.ID
	m.Response = true
	m.Opcode = r.Opcode
	m.RecursionDesired = r.RecursionDesired
	m.RecursionAvailable = true
	m.Rcode = rcode
	m.Question = r.Question
	if err := m.Pack(); err != nil {
		return nil
	}
	return m.Data
}

func (s *Server) serveDNS(proto string) func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
	return func(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) {
		out := s.answer(ctx, proto, r)
		if out == nil {
			return
		}
		// Msg.WriteTo handles UDP session routing and TCP length prefixing;
		// writing raw bytes to w does not.
		resp := &dns.Msg{Data: out}
		if _, err := resp.WriteTo(w); err != nil {
			s.log.Debug("write response failed", "proto", proto, "err", err)
		}
	}
}

func (s *Server) serveDoH(w http.ResponseWriter, req *http.Request) {
	var body []byte
	var err error
	switch req.Method {
	case http.MethodPost:
		body, err = io.ReadAll(io.LimitReader(req.Body, 65536))
	case http.MethodGet:
		body, err = base64.RawURLEncoding.DecodeString(req.URL.Query().Get("dns"))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err != nil || len(body) == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	r := &dns.Msg{Data: body}
	if err := r.Unpack(); err != nil {
		http.Error(w, "bad dns message", http.StatusBadRequest)
		return
	}
	out := s.answer(req.Context(), "doh", r)
	if out == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/dns-message")
	_, _ = w.Write(out)
}
