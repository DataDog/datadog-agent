package modules

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SecurityRuntime - Security runtime Factory
var SecurityRuntime = api.Factory{
	Name: "security_runtime",
	Fn: func(cfg *config.AgentConfig) (api.Module, error) {
		module, err := secmodule.NewModule(cfg)
		if err == ebpf.ErrNotImplemented {
			log.Info("Datadog runtime security agent is only supported on Linux")
			return nil, api.ErrNotEnabled
		}
		return module, err
	},
}
