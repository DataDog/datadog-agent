// +build linux

package modules

import (
	"fmt"
	"net/http"
	"os"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/elastic/go-libaudit"
	"github.com/pkg/errors"
)

// LinuxAuditProbe Factory
var LinuxAuditProbe = api.Factory{
	Name: "linux_audit_probe",
	Fn: func(cfg *config.AgentConfig) (api.Module, error) {
		if !cfg.CheckIsEnabled("Linux Audit") {
			log.Info("Linux Audit probe disabled")
			return nil, api.ErrNotEnabled
		}

		log.Infof("Starting the Linux Audit probe")
		if os.Geteuid() != 0 {
			return nil, errors.New("you must be root to receive audit data")
		}

		client, err := libaudit.NewMulticastAuditClient(nil)
		if err != nil {
			return nil, err
		}

		return &linuxAuditModule{client}, nil
	},
}

var _ api.Module = &linuxAuditModule{}

type linuxAuditModule struct {
	client *libaudit.AuditClient
}

func (o *linuxAuditModule) Close() {
	o.client.Close()
}

func (o *linuxAuditModule) Register(httpMux *http.ServeMux) error {
	httpMux.HandleFunc("/check/linux_audit", func(w http.ResponseWriter, req *http.Request) {
		status, err := o.client.GetStatus()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to obtain Linux Audit status: %s", err), http.StatusInternalServerError)
		}

		utils.WriteAsJSON(w, status)
	})

	return nil
}

func (o *linuxAuditModule) GetStats() map[string]interface{} {
	return nil
}
