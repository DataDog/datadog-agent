package trace

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type ServerlessTraceAgent struct {
	ta *agent.Agent
}

func (c *ServerlessTraceAgent) Start(datadogConfigPath string, context context.Context, cancel context.CancelFunc, waitingChan chan bool) {
	tc, confErr := config.Load(datadogConfigPath)
	tc.Hostname = ""
	tc.SynchronousFlushing = true
	if confErr != nil {
		log.Errorf("Unable to load trace agent config: %s", confErr)
	} else {
		c.ta = agent.NewAgent(context, tc)
		go func() {
			c.ta.Run()
		}()
		waitingChan <- true
	}
}

func (c *ServerlessTraceAgent) Get() *agent.Agent {
	return c.ta
}
