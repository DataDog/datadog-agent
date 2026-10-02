// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package status

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/util/procfilestats"
)

// TestToDictionaryNetwork exercises the TCP and UDP branches of toDictionary, asserting that each
// optional key is present (with the right value) when its field is set and absent when it is not.
// This pins down the conditionals that decide whether TLS, Format, AllowedIPs and DeniedIPs appear.
func TestToDictionaryNetwork(t *testing.T) {
	b := &Builder{}

	for _, networkType := range []string{config.TCPType, config.UDPType} {
		t.Run(networkType+"/all fields set", func(t *testing.T) {
			c := &config.LogsConfig{
				Type:       networkType,
				Service:    "svc",
				Source:     "src",
				Port:       8080,
				Format:     config.SyslogFormat,
				AllowedIPs: config.StringSliceField{"10.0.0.1", "10.0.0.2"},
				DeniedIPs:  config.StringSliceField{"192.168.0.1"},
			}
			if networkType == config.TCPType {
				c.TLS = &config.TLSListenerConfig{}
			}
			d := b.toDictionary(c)

			assert.Equal(t, "svc", d["Service"])
			assert.Equal(t, "src", d["Source"])
			assert.Equal(t, 8080, d["Port"])
			assert.Equal(t, config.SyslogFormat, d["Format"])
			assert.Equal(t, "10.0.0.1, 10.0.0.2", d["AllowedIPs"])
			assert.Equal(t, "192.168.0.1", d["DeniedIPs"])
			if networkType == config.TCPType {
				assert.Equal(t, "true", d["TLS"])
			}
		})

		t.Run(networkType+"/optional fields empty", func(t *testing.T) {
			c := &config.LogsConfig{
				Type:    networkType,
				Service: "svc",
				Source:  "src",
				Port:    8080,
				// Format empty, no AllowedIPs/DeniedIPs, no TLS.
			}
			d := b.toDictionary(c)

			assert.Equal(t, 8080, d["Port"])
			assert.NotContains(t, d, "TLS")
			assert.NotContains(t, d, "Format")
			assert.NotContains(t, d, "AllowedIPs")
			assert.NotContains(t, d, "DeniedIPs")
		})
	}
}

// TestToDictionaryTCPTLSOnly asserts TLS appears only when the TLS config is non-nil — the TCP-only
// conditional that the UDP case does not have.
func TestToDictionaryTCPTLSOnly(t *testing.T) {
	b := &Builder{}

	withTLS := b.toDictionary(&config.LogsConfig{Type: config.TCPType, Service: "s", Port: 1, TLS: &config.TLSListenerConfig{}})
	assert.Equal(t, "true", withTLS["TLS"])

	withoutTLS := b.toDictionary(&config.LogsConfig{Type: config.TCPType, Service: "s", Port: 1})
	assert.NotContains(t, withoutTLS, "TLS")
}

// TestToDictionaryDropsEmptyStrings asserts the final pass removes keys whose value is an empty
// string (and keeps non-empty ones), without removing non-string values like Port.
func TestToDictionaryDropsEmptyStrings(t *testing.T) {
	b := &Builder{}

	d := b.toDictionary(&config.LogsConfig{Type: config.TCPType, Service: "", Source: "src", Port: 0})
	assert.NotContains(t, d, "Service", "empty Service should be dropped")
	assert.Equal(t, "src", d["Source"], "non-empty Source should be kept")
	assert.Contains(t, d, "Port", "Port is an int and must not be dropped even when 0")
	assert.Equal(t, 0, d["Port"])
}

// TestGetProcessFileStatsPopulatedOnSuccess asserts that when process file stats are available,
// getProcessFileStats returns them (not an empty map) — pinning the err==nil fall-through path.
// Skips on platforms where the underlying call is unavailable so the test never flakes.
func TestGetProcessFileStatsPopulatedOnSuccess(t *testing.T) {
	if _, err := procfilestats.GetProcessFileStats(); err != nil {
		t.Skipf("process file stats unavailable on this platform: %v", err)
	}

	stats := (&Builder{}).getProcessFileStats()
	assert.Contains(t, stats, "CoreAgentProcessOpenFiles")
	assert.Contains(t, stats, "OSFileLimit")
}
