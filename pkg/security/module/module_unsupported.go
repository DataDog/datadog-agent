// +build !linux_bpf

package module

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	aconfig "github.com/DataDog/datadog-agent/pkg/process/config"
)

// NewModule instantiates a runtime security system-probe module
func NewModule(cfg *aconfig.AgentConfig) (api.Module, error) {
	return nil, ebpf.ErrNotImplemented
}
