// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package agent

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/flags"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/osutil"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const messageAgentDisabled = `trace-agent not enabled. Set the environment variable
DD_APM_ENABLED=true or add "apm_config.enabled: true" entry
to your datadog.yaml. Exiting...`

// Run is the entrypoint of our code, which starts the agent.
func Run(ctx context.Context) {
	if flags.Version {
		fmt.Print(info.VersionString())
		return
	}

	cfg, err := config.Load(flags.ConfigPath)
	if err != nil {
		if err == config.ErrMissingAPIKey {
			fmt.Println(config.ErrMissingAPIKey)

			// a sleep is necessary to ensure that supervisor registers this process as "STARTED"
			// If the exit is "too quick", we enter a BACKOFF->FATAL loop even though this is an expected exit
			// http://supervisord.org/subprocess.html#process-states
			time.Sleep(5 * time.Second)

			// Don't use os.Exit() method here, even with os.Exit(0) the Service Control Manager
			// on Windows will consider the process failed and log an error in the Event Viewer and
			// attempt to restart the process.
			return
		}
		osutil.Exitf("%v", err)
	}
	err = info.InitInfo(cfg) // for expvar & -info option
	if err != nil {
		osutil.Exitf("%v", err)
	}

	if flags.Info {
		if err := info.Info(os.Stdout, cfg); err != nil {
			osutil.Exitf("Failed to print info: %s", err)
		}
		return
	}

	if err := coreconfig.SetupLogger(
		coreconfig.LoggerName("TRACE"),
		cfg.LogLevel,
		cfg.LogFilePath,
		coreconfig.GetSyslogURI(),
		coreconfig.Datadog.GetBool("syslog_rfc"),
		coreconfig.Datadog.GetBool("log_to_console"),
		coreconfig.Datadog.GetBool("log_format_json"),
	); err != nil {
		osutil.Exitf("Cannot create logger: %v", err)
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

	if flags.CPUProfile != "" {
		f, err := os.Create(flags.CPUProfile)
		if err != nil {
			log.Error(err)
		}
		pprof.StartCPUProfile(f)
		log.Info("CPU profiling started...")
		defer pprof.StopCPUProfile()
	}

	if flags.PIDFilePath != "" {
		err := pidfile.WritePID(flags.PIDFilePath)
		if err != nil {
			log.Criticalf("Error writing PID file, exiting: %v", err)
			os.Exit(1)
		}

		log.Infof("PID '%d' written to PID file '%s'", os.Getpid(), flags.PIDFilePath)
		defer os.Remove(flags.PIDFilePath)
	}

	// set core limits as soon as possible
	if err := util.SetCoreLimit(); err != nil {
		log.Infof("Can't set core size limit: %v, core dumps might not be available after a crash", err)
	}

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

	// collect memory profile
	if flags.MemProfile != "" {
		f, err := os.Create(flags.MemProfile)
		if err != nil {
			log.Error("Could not create memory profile: ", err)
		}

		// get up-to-date statistics
		runtime.GC()
		// Not using WriteHeapProfile but instead calling WriteTo to
		// make sure we pass debug=1 and resolve pointers to names.
		if err := pprof.Lookup("heap").WriteTo(f, 1); err != nil {
			log.Error("Could not write memory profile: ", err)
		}
		f.Close()
	}
}
