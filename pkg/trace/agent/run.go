package agent

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	log "github.com/cihub/seelog"

	"github.com/StackVista/stackstate-agent/pkg/pidfile"
	"github.com/StackVista/stackstate-agent/pkg/trace/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/flags"
	"github.com/StackVista/stackstate-agent/pkg/trace/info"
	"github.com/StackVista/stackstate-agent/pkg/trace/metrics"
	"github.com/StackVista/stackstate-agent/pkg/trace/osutil"
	"github.com/StackVista/stackstate-agent/pkg/trace/watchdog"
)

const agentDisabledMessage = `trace-agent not enabled.
Set env var STS_APM_ENABLED=true or add
apm_enabled: true
to your datadog.conf file.
Exiting.`

// Run is the entrypoint of our code, which starts the agent.
func Run(ctx context.Context) {
	// configure a default logger before anything so we can observe initialization
	if flags.Info || flags.Version {
		log.UseLogger(log.Disabled)
	} else {
		SetupDefaultLogger()
		defer log.Flush()
	}

	defer watchdog.LogOnPanic()

	// start CPU profiling
	if flags.CPUProfile != "" {
		f, err := os.Create(flags.CPUProfile)
		if err != nil {
			log.Critical(err)
		}
		pprof.StartCPUProfile(f)
		log.Info("CPU profiling started...")
		defer pprof.StopCPUProfile()
	}

	if flags.Version {
		fmt.Print(info.VersionString())
		return
	}

	if !flags.Info && flags.PIDFilePath != "" {
		err := pidfile.WritePID(flags.PIDFilePath)
		if err != nil {
			log.Errorf("Error while writing PID file, exiting: %v", err)
			os.Exit(1)
		}

		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), flags.PIDFilePath)
		defer func() {
			// remove pidfile if set
			os.Remove(flags.PIDFilePath)
		}()
	}

	cfg, err := config.Load(flags.ConfigPath)
	if err != nil {
		osutil.Exitf("%v", err)
	}
	err = info.InitInfo(cfg) // for expvar & -info option
	if err != nil {
		panic(err)
	}

	if flags.Info {
		if err := info.Info(os.Stdout, cfg); err != nil {
			os.Stdout.WriteString(fmt.Sprintf("failed to print info: %s\n", err))
			os.Exit(1)
		}
		return
	}

	// Exit if tracing is not enabled
	if !cfg.Enabled {
		log.Info(agentDisabledMessage)

		// a sleep is necessary to ensure that supervisor registers this process as "STARTED"
		// If the exit is "too quick", we enter a BACKOFF->FATAL loop even though this is an expected exit
		// http://supervisord.org/subprocess.html#process-states
		time.Sleep(5 * time.Second)
		return
	}

	// Initialize logging (replacing the default logger). No need
	// to defer log.Flush, it was already done when calling
	// "SetupDefaultLogger" earlier.
	cfgLogLevel := strings.ToLower(cfg.LogLevel)
	if cfgLogLevel == "warning" {
		// to match core agent:
		// https://github.com/DataDog/datadog-agent/blob/6f2d901aeb19f0c0a4e09f149c7cc5a084d2f708/pkg/config/log.go#L74-L76
		cfgLogLevel = "warn"
	}
	logLevel, ok := log.LogLevelFromString(cfgLogLevel)
	if !ok {
		logLevel = log.InfoLvl
	}
	duration := 10 * time.Second
	if !cfg.LogThrottlingEnabled {
		duration = 0
	}
	err = SetupLogger(logLevel, cfg.LogFilePath, duration, 10)
	if err != nil {
		osutil.Exitf("cannot create logger: %v", err)
	}

	// Initialize dogstatsd client
	err = metrics.Configure(cfg, []string{"version:" + info.Version})
	if err != nil {
		osutil.Exitf("cannot configure dogstatsd: %v", err)
	}

	// count the number of times the agent started
	metrics.Count("datadog.trace_agent.started", 1, nil, 1)

	// Seed rand
	rand.Seed(time.Now().UTC().UnixNano())

	ta := NewAgent(ctx, cfg)

	log.Infof("trace-agent running on host %s", cfg.Hostname)
	ta.Run()

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
