// +build !linux

package checks

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/containers"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// RTContainer is a singleton RTContainerCheck.
var RTContainer = &RTContainerCheck{}

// RTContainerCheck collects numeric statistics about live containers.
type RTContainerCheck struct {
	sysInfo *model.SystemInfo
	lastRun time.Time
}

// Init initializes a RTContainerCheck instance.
func (r *RTContainerCheck) Init(cfg *config.AgentConfig, sysInfo *model.SystemInfo) {
	r.sysInfo = sysInfo
}

// Name returns the name of the RTContainerCheck.
func (r *RTContainerCheck) Name() string { return "rtcontainer" }

// Endpoint returns the endpoint where this check is submitted.
func (r *RTContainerCheck) Endpoint() string { return "/api/v1/container" }

// RealTime indicates if this check only runs in real-time mode.
func (r *RTContainerCheck) RealTime() bool { return true }

// Run runs the real-time container check getting container-level stats from the Cgroups and Docker APIs.
func (r *RTContainerCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	return nil, nil
}

// fmtContainerStats formats and chunks the containers into a slice of chunks using a specific
// number of chunks. len(result) MUST EQUAL chunks.
func fmtContainerStats(
	ctrList []*containers.Container,
	lastRates map[string]util.ContainerRateMetrics,
	lastRun time.Time,
	chunks int,
) [][]*model.ContainerStat {
	return make([][]*model.ContainerStat, chunks)
}
