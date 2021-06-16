// +build linux

package modules

import (
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
)

// OOMKillProbe Factory
var OOMKillProbe = module.Factory{
	Name: config.OOMKillProbeModule,
	Fn: func(cfg *config.Config) (module.Module, error) {
		log.Infof("Starting the OOM Kill probe")
		okp, err := probe.NewOOMKillProbe(ebpf.NewConfig())
		if err != nil {
			return nil, errors.Wrapf(err, "unable to start the OOM kill probe")
		}
		return &oomKillModule{okp}, nil
	},
}

var _ module.Module = &oomKillModule{}

type oomKillModule struct {
	*probe.OOMKillProbe
}

func (o *oomKillModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check/oom_kill", func(w http.ResponseWriter, req *http.Request) {
		stats := o.OOMKillProbe.GetAndFlush()
		utils.WriteAsJSON(w, stats)
	})

	return nil
}

func (o *oomKillModule) GetStats() map[string]interface{} {
	return nil
}
