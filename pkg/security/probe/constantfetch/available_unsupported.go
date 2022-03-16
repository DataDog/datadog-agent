//go:build linux && !linux_bpf
// +build linux,!linux_bpf

package constantfetch

import (
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-go/statsd"
)

// GetAvailableConstantFetchers returns available constant fetchers
func GetAvailableConstantFetchers(config *config.Config, kv *kernel.Version, statsdClient *statsd.Client) []ConstantFetcher {
	fetchers := make([]ConstantFetcher, 0)

	btfhubFetcher, err := NewBTFHubConstantFetcher(kv)
	if err != nil {
		log.Errorf("failed to create btfhub constant fetcher: %v", err)
	} else {
		fetchers = append(fetchers, btfhubFetcher)
	}

	fallbackConstantFetcher := NewFallbackConstantFetcher(kv)
	fetchers = append(fetchers, fallbackConstantFetcher)

	return fetchers
}
