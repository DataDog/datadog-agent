// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"

	ddConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServerlessTraceAgent represents a trace agent in a serverless context
type ServerlessTraceAgent struct {
	ta           *agent.Agent
	spanModifier *spanModifier
	cancel       context.CancelFunc
}

// Load abstracts the file configuration loading
type Load interface {
	Load() (*config.AgentConfig, error)
}

// LoadConfig is implementing Load to retrieve the config
type LoadConfig struct {
	Path string
}

// Load loads the config from a file path
func (l *LoadConfig) Load() (*config.AgentConfig, error) {
	return config.Load(l.Path)
}

// Start starts the agent
func (s *ServerlessTraceAgent) Start(enabled bool, loadConfig Load) {
	if enabled {
		// during hostname resolution the first step is to make a GRPC call which is timeboxed to a 2 seconds deadline
		// in the serverless mode, we don't start the GRPC server so this call will fail and cause a 2 seconds delay
		// by setting cmd_port to -1, this will cause the GRPC client to fail instantly
		ddConfig.Datadog.Set("cmd_port", "-1")

		// make sure we blocklist /hello and /flush calls
		userProvidedBlocklist := []string{}
		// check if ignore_resources is set before casting to string slice to avoid logging a warning
		if ddConfig.Datadog.IsSet("apm_config.ignore_resources") {
			userProvidedBlocklist = ddConfig.Datadog.GetStringSlice("apm_config.ignore_resources")
		}
		ddConfig.Datadog.Set("apm_config.ignore_resources", buildTraceBlocklist(userProvidedBlocklist))

		tc, confErr := loadConfig.Load()
		if confErr != nil {
			log.Errorf("Unable to load trace agent config: %s", confErr)
		} else {
			context, cancel := context.WithCancel(context.Background())
			tc.Hostname = ""
			tc.SynchronousFlushing = true
			s.ta = agent.NewAgent(context, tc)
			s.spanModifier = &spanModifier{}
			s.ta.ModifySpan = s.spanModifier.ModifySpan
			s.cancel = cancel
			go func() {
				s.ta.Run()
			}()
		}
	}
}

// Get returns the trace agent instance
func (s *ServerlessTraceAgent) Get() *agent.Agent {
	return s.ta
}

// SetTags sets the tags to the trace agent config and span processor
func (s *ServerlessTraceAgent) SetTags(tagMap map[string]string) {
	s.ta.SetGlobalTagsUnsafe(tagMap)
	s.spanModifier.tags = tagMap
}

// Stop stops the trace agent
func (s *ServerlessTraceAgent) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

func buildTraceBlocklist(userProvidedList []string) []string {
	list := append(userProvidedList, "GET /lambda/hello")
	list = append(list, "POST /lambda/flush")
	return list
}
