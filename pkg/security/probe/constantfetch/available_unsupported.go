// +build linux,!linux_bpf

package constantfetch

import (
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-go/statsd"
)

// GetAvailableConstantFetchers returns available constant fetchers
func GetAvailableConstantFetchers(config *config.Config, kv *kernel.Version, statsdClient *statsd.Client) []ConstantFetcher {
	fallbackConstantFetcher := NewFallbackConstantFetcher(kv)
	return []ConstantFetcher{
		fallbackConstantFetcher,
	}
}
