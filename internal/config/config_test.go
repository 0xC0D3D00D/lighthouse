package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0xc0d3d00d/lighthouse/internal/config"
)

func TestParsedBackendsSuffix(t *testing.T) {
	cfg := config.Config{Backends: []string{
		"udp://1.1.1.1:53",
		"udp://10.96.0.10:53?suffix=cluster.local.",
		"tls://8.8.8.8?suffix=.Corp.Example", // leading dot, no trailing dot, mixed case
		"https://dns.google/dns-query?suffix=internal.",
	}}
	ups, err := cfg.ParsedBackends()
	require.NoError(t, err)
	require.Len(t, ups, 4)

	assert.Equal(t, ".", ups[0].Suffix)
	assert.Equal(t, "cluster.local.", ups[1].Suffix)
	assert.Equal(t, "corp.example.", ups[2].Suffix)
	assert.Equal(t, "8.8.8.8:853", ups[2].Address)
	assert.Equal(t, "internal.", ups[3].Suffix)
	assert.Equal(t, "https://dns.google/dns-query", ups[3].URL, "suffix param must be stripped from the DoH endpoint")
}

func TestParsedBackendsInvalidSuffix(t *testing.T) {
	cfg := config.Config{Backends: []string{"udp://1.1.1.1:53?suffix=foo..bar."}}
	_, err := cfg.ParsedBackends()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty label")
}
