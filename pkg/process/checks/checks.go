package checks

import (
	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

// Check is an interface for Agent checks that collect data. Each check returns
// a specific MessageBody type that will be published to the intake endpoint or
// processed in another way (e.g. printed for debugging).
// Before checks are used you must called Init.
type Check interface {
	Init(cfg *config.AgentConfig, info *model.SystemInfo)
	Name() string
	Endpoint() string
	RealTime() bool
	Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error)
}

// All is all the singleton check instances.
var All = []Check{
	Process,
	RTProcess,
	Container,
	RTContainer,
	Connections,
	Pod,
}
