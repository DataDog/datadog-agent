package modules

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
)

// SecurityRuntime - Security runtime Factory
var SecurityRuntime = api.Factory{
	Name: "security_runtime",
	Fn: func(cfg *config.AgentConfig) (api.Module, error) {
		return secmodule.NewModule(cfg)
	},
}

var _ api.Module = &secmodule.Module{}
