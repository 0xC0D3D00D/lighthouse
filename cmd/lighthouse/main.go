package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"codeberg.org/miekg/dns"

	"github.com/0xc0d3d00d/lighthouse/internal/cache"
	"github.com/0xc0d3d00d/lighthouse/internal/config"
	"github.com/0xc0d3d00d/lighthouse/internal/dnsserver"
	"github.com/0xc0d3d00d/lighthouse/internal/health"
	"github.com/0xc0d3d00d/lighthouse/internal/httpserver/api"
	"github.com/0xc0d3d00d/lighthouse/internal/httpserver/ops"
	"github.com/0xc0d3d00d/lighthouse/internal/metrics"
	"github.com/0xc0d3d00d/lighthouse/internal/recordbook"
	"github.com/0xc0d3d00d/lighthouse/internal/resolver"
	resolvermodel "github.com/0xc0d3d00d/lighthouse/internal/resolver/model"
	"github.com/0xc0d3d00d/lighthouse/internal/resolver/backend"
	"github.com/0xc0d3d00d/lighthouse/internal/stats"
	"github.com/0xc0d3d00d/lighthouse/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func logLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(cfg.LogLevel)}))
	slog.SetDefault(log)

	cfgUps, err := cfg.ParsedBackends()
	if err != nil {
		return err
	}
	ups := make([]resolvermodel.Backend, 0, len(cfgUps))
	for _, u := range cfgUps {
		ups = append(ups, resolvermodel.Backend{Scheme: u.Scheme, Address: u.Address, URL: u.URL, Suffix: u.Suffix})
	}

	// Services.
	cacheSvc, err := cache.New(cfg.CacheMaxSize)
	if err != nil {
		return err
	}
	defer cacheSvc.Close()

	bookSvc, err := recordbook.New(cfg.RecordBookPath)
	if err != nil {
		return err
	}
	defer bookSvc.Close()

	metrics.RegisterCacheSize(func() float64 { return float64(cacheSvc.Len()) })
	metrics.RegisterRecordBookSize(func() float64 { return float64(bookSvc.Count()) })

	statsSvc := stats.New(time.Hour)

	// The resolver and health services depend on each other (resolver asks
	// health for the mode; health probes the beacon through the resolver), so
	// the health adapter is bound after both are constructed.
	healthRef := &lazyHealth{}
	resolverSvc := resolver.New(
		ups,
		backend.NewClient(cfg.BackendTimeout),
		cacheSvc,
		bookSvc,
		healthRef,
		cfg.StoreNegative,
		log,
	)

	beaconType := dns.StringToType[strings.ToUpper(cfg.BeaconType)]
	if beaconType == 0 {
		beaconType = dns.TypeA
	}
	healthSvc := health.New(resolverSvc, health.Options{
		BeaconName:   fqdn(cfg.BeaconName),
		BeaconType:   beaconType,
		Interval:     cfg.BeaconInterval,
		FailAfter:    cfg.BeaconFailAfter,
		RecoverAfter: cfg.BeaconRecoverAfter,
	}, log)
	healthRef.bind(healthSvc)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go healthSvc.Run(ctx)

	errCh := make(chan error, 8)
	var ready atomic.Bool

	// DNS listeners.
	dnsSrv := dnsserver.New(resolverSvc, statsSvc, dnsserver.Options{
		UDPAddr: cfg.ListenUDP,
		TCPAddr: cfg.ListenTCP,
		DoTAddr: cfg.ListenDoT,
		DoHAddr: cfg.ListenDoH,
		TLSCert: cfg.TLSCert,
		TLSKey:  cfg.TLSKey,
	}, log)
	if err := dnsSrv.Start(errCh); err != nil {
		return err
	}

	// Ops HTTP server: /metrics, /healthz, /readyz.
	opsSrv := ops.New(cfg.OpsAddr, ready.Load)
	go func() {
		log.Info("ops http server starting", "addr", cfg.OpsAddr)
		if err := opsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Dashboard HTTP server: SPA + /api.
	webFS := web.FS()
	if webFS == nil {
		log.Warn("dashboard assets not embedded; serving API only")
	}
	apiSrv := api.New(cfg.HTTPAddr, healthSvc, statsSvc, bookSvc, resolverSvc, cacheSvc, webFS, log)
	go func() {
		log.Info("dashboard http server starting", "addr", cfg.HTTPAddr)
		if err := apiSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	ready.Store(true)
	log.Info("lighthouse started", "backends", len(ups), "beacon", cfg.BeaconName)

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		log.Error("server error, shutting down", "err", err)
	}
	ready.Store(false)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dnsSrv.Shutdown(shutdownCtx)
	_ = apiSrv.Shutdown(shutdownCtx)
	_ = opsSrv.Shutdown(shutdownCtx)
	return nil
}

func fqdn(name string) string {
	if strings.HasSuffix(name, ".") {
		return name
	}
	return name + "."
}

// lazyHealth breaks the resolver<->health construction cycle.
type lazyHealth struct {
	impl resolver.Health
}

func (l *lazyHealth) bind(h resolver.Health) { l.impl = h }

func (l *lazyHealth) Survival() bool {
	if l.impl == nil {
		return false
	}
	return l.impl.Survival()
}
