package modules

import (
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OOMKillProbe Factory
var OOMKillProbe = api.Factory{
	Name: "oom_kill_probe",
	Fn: func(cfg *config.AgentConfig) (api.Module, error) {
		if !cfg.CheckIsEnabled(config.OOMKillCheckName) {
			log.Info("OOM kill probe disabled")
			return nil, api.ErrNotEnabled
		}

		log.Infof("Starting the OOM Kill probe")
		okp, err := probe.NewOOMKillProbe(ebpf.SysProbeConfigFromConfig(cfg))
		if err != nil {
			return nil, fmt.Errorf("unable to start the OOM kill probe: %w", err)
		}
		return &oomKillModule{okp}, nil
	},
}

var _ api.Module = &oomKillModule{}

type oomKillModule struct {
	*probe.OOMKillProbe
}

func (o *oomKillModule) Register(httpMux *http.ServeMux) error {
	httpMux.HandleFunc("/check/oom_kill", func(w http.ResponseWriter, req *http.Request) {
		stats := o.OOMKillProbe.GetAndFlush()
		utils.WriteAsJSON(w, stats)
	})

	return nil
}

func (o *oomKillModule) GetStats() map[string]interface{} {
	return nil
}
