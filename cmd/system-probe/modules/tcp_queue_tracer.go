package modules

import (
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

// TCPQueueLength Factory
var TCPQueueLength = api.Factory{
	Name: "tcp_queue_length_tracer",
	Fn: func(cfg *config.AgentConfig) (api.Module, error) {
		if !cfg.CheckIsEnabled("TCP queue length") {
			return nil, api.ErrNotEnabled
		}

		t, err := ebpf.NewTCPQueueLengthTracer()
		return &tcpQueueLengthModule{t}, err
	},
}

var _ api.Module = &tcpQueueLengthModule{}

type tcpQueueLengthModule struct {
	*ebpf.TCPQueueLengthTracer
}

func (t *tcpQueueLengthModule) Register(httpMux *http.ServeMux) error {
	httpMux.HandleFunc("/check/tcp_queue_length", func(w http.ResponseWriter, req *http.Request) {
		stats := t.TCPQueueLengthTracer.GetAndFlush()
		utils.WriteAsJSON(w, stats)
	})

	return nil
}

func (t *tcpQueueLengthModule) GetStats() map[string]interface{} {
	return nil
}
