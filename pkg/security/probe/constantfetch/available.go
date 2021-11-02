// +build linux,linux_bpf

package constantfetch

import (
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
)

// GetAvailableConstantFetchers returns available constant fetchers
func GetAvailableConstantFetchers(config *config.Config, kv *kernel.Version) []ConstantFetcher {
	rcConstantFetcher := NewRuntimeCompilationConstantFetcher(&config.Config)
	fallbackConstantFetcher := NewFallbackConstantFetcher(kv)
	return []ConstantFetcher{
		rcConstantFetcher,
		fallbackConstantFetcher,
	}
}
