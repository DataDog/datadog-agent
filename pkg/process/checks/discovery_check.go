package checks

import (
	"os"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var Discovery = &DiscoveryCheck{}

type DiscoveryCheck struct {
	probe *procutil.Probe
	info  *model.SystemInfo
}

func (d *DiscoveryCheck) Init(cfg *config.AgentConfig, info *model.SystemInfo) {
	d.probe = procutil.NewProcessProbe()
	d.info = info
}

func (d *DiscoveryCheck) Name() string { return config.DiscoveryCheckName }

func (d *DiscoveryCheck) RealTime() bool { return false }

func (d *DiscoveryCheck) Run(_ *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	log.Info("Running process discovery check")
	hostname, err := os.Hostname()
	if err != nil {
		_ = log.Warn("unable to get hostname")
		hostname = "unknown"
	}

	procs, err := getAllProcesses(d.probe)
	if err != nil {
		return nil, log.Error(err)
	}

	payload := model.CollectorProcDiscovery{
		HostName:           hostname,
		GroupId:            groupID,
		GroupSize:          0,
		ProcessDiscoveries: make([]*model.ProcessDiscovery, 0, len(procs)),
		Host: &model.Host{
			Name:        hostname,
			NumCpus:     d.info.Cpus[0].Number,
			TotalMemory: d.info.TotalMemory,
		},
	}
	for _, proc := range procs {
		payload.ProcessDiscoveries = append(payload.ProcessDiscoveries, &model.ProcessDiscovery{
			Pid:     proc.Pid,
			NsPid:   proc.NsPid,
			Host:    payload.Host,
			Command: formatCommand(proc),
			User:    formatUser(proc),
		})
		payload.GroupSize += 1
	}

	return []model.MessageBody{&payload}, nil
}
