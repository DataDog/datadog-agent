package agent

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/flags"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/osutil"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const messageAgentDisabled = `trace-agent not enabled. Set the environment variable
DD_APM_ENABLED=true or add "apm_config.enabled: true" entry
to your datadog.yaml. Exiting...`

// Run is the entrypoint of our code, which starts the agent.
func Run(ctx context.Context) {
	cfg, err := config.Load(flags.ConfigPath)
	if err != nil {
		osutil.Exitf("%v", err)
	}
	err = info.InitInfo(cfg) // for expvar & -info option
	if err != nil {
		panic(err)
	}

	if flags.Version {
		fmt.Print(info.VersionString())
		return
	}

	if flags.Info {
		if err := info.Info(os.Stdout, cfg); err != nil {
			osutil.Exitf("failed to print info: %s\n", err)
		}
		return
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

	if flags.CPUProfile != "" {
		f, err := os.Create(flags.CPUProfile)
		if err != nil {
			log.Critical(err)
		}
		pprof.StartCPUProfile(f)
		log.Info("CPU profiling started...")
		defer pprof.StopCPUProfile()
	}

	if flags.PIDFilePath != "" {
		err := pidfile.WritePID(flags.PIDFilePath)
		if err != nil {
			log.Criticalf("error writing PID file, exiting: %v", err)
			os.Exit(1)
		}

		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), flags.PIDFilePath)
		defer os.Remove(flags.PIDFilePath)
	}

	err = metrics.Configure(cfg, []string{"version:" + info.Version})
	if err != nil {
		osutil.Exitf("cannot configure dogstatsd: %v", err)
	}
	metrics.Count("datadog.trace_agent.started", 1, nil, 1)

	// Seed rand
	rand.Seed(time.Now().UTC().UnixNano())

	agnt := NewAgent(ctx, cfg)
	log.Infof("trace-agent running on host %s", cfg.Hostname)
	agnt.Run()

	// collect memory profile
	if flags.MemProfile != "" {
		f, err := os.Create(flags.MemProfile)
		if err != nil {
			log.Critical("could not create memory profile: ", err)
		}

		// get up-to-date statistics
		runtime.GC()
		// Not using WriteHeapProfile but instead calling WriteTo to
		// make sure we pass debug=1 and resolve pointers to names.
		if err := pprof.Lookup("heap").WriteTo(f, 1); err != nil {
			log.Critical("could not write memory profile: ", err)
		}
		f.Close()
	}
}
