//go:build linux && linux_bpf
// +build linux,linux_bpf

package constantfetch

import (
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// GetAvailableConstantFetchers returns available constant fetchers
func GetAvailableConstantFetchers(config *config.Config, kv *kernel.Version, statsdClient *statsd.Client) []ConstantFetcher {
	fetchers := make([]ConstantFetcher, 0)

	if coreFetcher, err := NewBTFConstantFetcherFromCurrentKernel(); err == nil {
		fetchers = append(fetchers, coreFetcher)
	}

	if config.EnableRuntimeCompiledConstants {
		rcConstantFetcher := NewRuntimeCompilationConstantFetcher(config, statsdClient)
		fetchers = append(fetchers, rcConstantFetcher)
	}

	btfhubFetcher, err := NewBTFHubConstantFetcher(kv)
	if err != nil {
		log.Debugf("failed to create btfhub constant fetcher: %v", err)
	} else {
		fetchers = append(fetchers, btfhubFetcher)
	}

	OffsetGuesserFetcher := NewOffsetGuesserFetcher(config)
	fetchers = append(fetchers, OffsetGuesserFetcher)

	fallbackConstantFetcher := NewFallbackConstantFetcher(kv)
	fetchers = append(fetchers, fallbackConstantFetcher)

	return fetchers
}
