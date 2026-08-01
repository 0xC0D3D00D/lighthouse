package config

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/caarlos0/env/v11"
)

// EnvPrefix is prepended to every configuration variable name.
const EnvPrefix = "LIGHTHOUSE_"

// Backend is a parsed backend server definition.
// Supported schemes: udp, tcp, tls (DoT), https (DoH).
type Backend struct {
	Scheme string
	// Address is host:port for udp/tcp/tls backends.
	Address string
	// URL is the full DoH endpoint for https backends.
	URL string
}

// Config is populated from LIGHTHOUSE_-prefixed environment variables.
type Config struct {
	// Backends is a comma separated list of backend URIs, e.g.
	// udp://1.1.1.1:53,tls://8.8.8.8:853,https://dns.google/dns-query
	Backends []string `env:"BACKENDS,required"`

	ListenUDP string `env:"LISTEN_UDP" envDefault:":53"`
	ListenTCP string `env:"LISTEN_TCP" envDefault:":53"`
	ListenDoT string `env:"LISTEN_DOT"`
	ListenDoH string `env:"LISTEN_DOH"`
	// TLSCert/TLSKey are required when ListenDoT or ListenDoH is set.
	TLSCert string `env:"TLS_CERT"`
	TLSKey  string `env:"TLS_KEY"`

	BackendTimeout time.Duration `env:"BACKEND_TIMEOUT" envDefault:"3s"`

	CacheMaxSize int `env:"CACHE_MAX_SIZE" envDefault:"100000"`

	RecordBookPath string `env:"RECORDBOOK_PATH" envDefault:"./data/recordbook"`
	// StoreNegative persists NXDOMAIN/empty answers in the record book.
	StoreNegative bool `env:"STORE_NEGATIVE" envDefault:"false"`

	// BeaconName is the record probed on all backends to detect connectivity.
	BeaconName     string        `env:"BEACON_NAME,required"`
	BeaconType     string        `env:"BEACON_TYPE" envDefault:"A"`
	BeaconInterval time.Duration `env:"BEACON_INTERVAL" envDefault:"10s"`
	// BeaconFailAfter: beacon unreachable on ALL backends for this long -> survival mode.
	BeaconFailAfter time.Duration `env:"BEACON_FAIL_AFTER" envDefault:"1m"`
	// BeaconRecoverAfter: beacon continuously reachable for this long -> back to normal mode.
	BeaconRecoverAfter time.Duration `env:"BEACON_RECOVER_AFTER" envDefault:"1m"`

	HTTPAddr string `env:"HTTP_ADDR" envDefault:":8080"`
	OpsAddr  string `env:"OPS_ADDR" envDefault:":9090"`

	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

func Load() (Config, error) {
	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Prefix: EnvPrefix}); err != nil {
		return cfg, fmt.Errorf("parse env: %w", err)
	}
	// envDefault also kicks in for explicitly-empty variables; an empty
	// listener address means "disabled", so honor it.
	if v, ok := os.LookupEnv(EnvPrefix + "LISTEN_UDP"); ok && v == "" {
		cfg.ListenUDP = ""
	}
	if v, ok := os.LookupEnv(EnvPrefix + "LISTEN_TCP"); ok && v == "" {
		cfg.ListenTCP = ""
	}
	if _, err := cfg.ParsedBackends(); err != nil {
		return cfg, err
	}
	if (cfg.ListenDoT != "" || cfg.ListenDoH != "") && (cfg.TLSCert == "" || cfg.TLSKey == "") {
		return cfg, fmt.Errorf("LIGHTHOUSE_TLS_CERT and LIGHTHOUSE_TLS_KEY are required for DoT/DoH listeners")
	}
	return cfg, nil
}

func (c Config) ParsedBackends() ([]Backend, error) {
	ups := make([]Backend, 0, len(c.Backends))
	for _, raw := range c.Backends {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("backend %q: %w", raw, err)
		}
		switch u.Scheme {
		case "udp", "tcp", "tls":
			host := u.Host
			if u.Port() == "" {
				port := "53"
				if u.Scheme == "tls" {
					port = "853"
				}
				host = fmt.Sprintf("%s:%s", u.Hostname(), port)
			}
			ups = append(ups, Backend{Scheme: u.Scheme, Address: host})
		case "https":
			ups = append(ups, Backend{Scheme: "https", URL: raw})
		default:
			return nil, fmt.Errorf("backend %q: unsupported scheme %q", raw, u.Scheme)
		}
	}
	if len(ups) == 0 {
		return nil, fmt.Errorf("no backends configured")
	}
	return ups, nil
}
