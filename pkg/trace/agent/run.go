// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package agent

import (
	"context"
	"math/rand"
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/osutil"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const messageAgentDisabled = `trace-agent not enabled. Set the environment variable
DD_APM_ENABLED=true or add "apm_config.enabled: true" entry
to your datadog.yaml. Exiting...`

// ConfigPath specifies the path to the configuration file. It is set via the
// command's -c / --cfgpath persistent flag.
var ConfigPath string

// Run is the entrypoint of our code, which starts the agent.
func Run(ctx context.Context) {
	cfg, err := config.Load(ConfigPath)
	if err != nil {
		osutil.Exitf("%v", err)
	}
	err = info.InitInfo(cfg) // for expvar & -info option
	if err != nil {
		osutil.Exitf("%v", err)
	}

	if err := setupLogger(cfg); err != nil {
		osutil.Exitf("cannot create logger: %v", err)
	}
	defer log.Flush()

	if !cfg.Enabled {
		log.Info(messageAgentDisabled)

		// a sleep is necessary to ensure that supervisor registers this process as "STARTED"
		// If the exit is "too quick", we enter a BACKOFF->FATAL loop even though this is an expected exit
		// http://supervisord.org/subprocess.html#process-states
		time.Sleep(5 * time.Second)
		return
	}

	defer watchdog.LogOnPanic()

	err = metrics.Configure(cfg, []string{"version:" + info.Version})
	if err != nil {
		osutil.Exitf("cannot configure dogstatsd: %v", err)
	}
	defer metrics.Flush()
	defer timing.Stop()

	metrics.Count("datadog.trace_agent.started", 1, nil, 1)

	rand.Seed(time.Now().UTC().UnixNano())

	tagger.Init()
	defer tagger.Stop()

	agnt := NewAgent(ctx, cfg)
	log.Infof("Trace agent running on host %s", cfg.Hostname)
	agnt.Run()
}
