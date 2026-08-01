package backend

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	"codeberg.org/miekg/dns"

	"github.com/0xc0d3d00d/lighthouse/internal/resolver/model"
)

// Client queries a single backend over its configured transport.
type Client struct {
	timeout time.Duration
	plain   *dns.Client
	tlsc    *dns.Client
	httpc   *http.Client
}

func NewClient(timeout time.Duration) *Client {
	plain := dns.NewClient()
	plain.Transport.ReadTimeout = timeout
	plain.Transport.WriteTimeout = timeout

	tlsc := dns.NewClient()
	tlsc.Transport.ReadTimeout = timeout
	tlsc.Transport.WriteTimeout = timeout
	tlsc.Transport.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}

	return &Client{
		timeout: timeout,
		plain:   plain,
		tlsc:    tlsc,
		httpc:   &http.Client{Timeout: timeout},
	}
}

// Query resolves name/qtype against the given backend and returns the response.
func (c *Client) Query(ctx context.Context, up model.Backend, name string, qtype uint16) (*dns.Msg, error) {
	m := dns.NewMsg(name, qtype)
	if m == nil {
		return nil, fmt.Errorf("invalid query %s/%d", name, qtype)
	}
	m.UDPSize = 1232

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var (
		resp *dns.Msg
		err  error
	)
	switch up.Scheme {
	case "udp":
		resp, _, err = c.plain.Exchange(ctx, m, "udp", up.Address)
		if err == nil && resp.Truncated {
			resp, _, err = c.plain.Exchange(ctx, m, "tcp", up.Address)
		}
	case "tcp":
		resp, _, err = c.plain.Exchange(ctx, m, "tcp", up.Address)
	case "tls":
		resp, _, err = c.tlsc.Exchange(ctx, m, "tcp", up.Address)
	case "https":
		resp, err = c.doh(ctx, up.URL, m)
	default:
		return nil, fmt.Errorf("unsupported backend scheme %q", up.Scheme)
	}
	if err != nil {
		return nil, err
	}
	if err := resp.Unpack(); err != nil {
		return nil, fmt.Errorf("unpack response: %w", err)
	}
	return resp, nil
}

func (c *Client) doh(ctx context.Context, endpoint string, m *dns.Msg) (*dns.Msg, error) {
	if len(m.Data) == 0 {
		if err := m.Pack(); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(m.Data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	res, err := c.httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("doh %s: status %d", endpoint, res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 65536))
	if err != nil {
		return nil, err
	}
	resp := &dns.Msg{Data: body}
	return resp, nil
}
