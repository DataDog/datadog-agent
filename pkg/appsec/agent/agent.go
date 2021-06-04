package agent

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

type Agent struct {

}

func NewAgent(ctx context.Context, conf *config.AgentConfig) *Agent {
}
