package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/heartbeat"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	ddutil "github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const loggerName ddconfig.LoggerName = "PROCESS"

var opts struct {
	configPath         string
	sysProbeConfigPath string
	pidfilePath        string
	debug              bool
	version            bool
	check              string
	info               bool
}

// version info sourced from build flags
var (
	Version   string
	GitCommit string
	GitBranch string
	BuildDate string
	GoVersion string
)

// versionString returns the version information filled in at build time
func versionString(sep string) string {
	var buf bytes.Buffer

	if Version != "" {
		fmt.Fprintf(&buf, "Version: %s%s", Version, sep)
	}
	if GitCommit != "" {
		fmt.Fprintf(&buf, "Git hash: %s%s", GitCommit, sep)
	}
	if GitBranch != "" {
		fmt.Fprintf(&buf, "Git branch: %s%s", GitBranch, sep)
	}
	if BuildDate != "" {
		fmt.Fprintf(&buf, "Build date: %s%s", BuildDate, sep)
	}
	if GoVersion != "" {
		fmt.Fprintf(&buf, "Go Version: %s%s", GoVersion, sep)
	}

	return buf.String()
}

const (
	agent6DisabledMessage = `process-agent not enabled.
Set env var DD_PROCESS_AGENT_ENABLED=true or add
process_config:
  enabled: "true"
to your datadog.yaml file.
Exiting.`
)

func runAgent(exit chan struct{}) {
	if opts.version {
		fmt.Print(versionString("\n"))
		cleanupAndExit(0)
	}

	// set core limits as soon as possible
	if err := ddutil.SetCoreLimit(); err != nil {
		log.Infof("Can't set core size limit: %v, core dumps might not be available after a crash", err)
	}

	if opts.check == "" && !opts.info && opts.pidfilePath != "" {
		err := pidfile.WritePID(opts.pidfilePath)
		if err != nil {
			log.Errorf("Error while writing PID file, exiting: %v", err)
			cleanupAndExit(1)
		}

		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), opts.pidfilePath)
		defer func() {
			// remove pidfile if set
			os.Remove(opts.pidfilePath)
		}()
	}

	cfg, err := config.NewAgentConfig(loggerName, opts.configPath, opts.sysProbeConfigPath)
	if err != nil {
		log.Criticalf("Error parsing config: %s", err)
		cleanupAndExit(1)
	}

	// Now that the logger is configured log host info
	hostInfo := host.GetStatusInformation()
	log.Infof("running on platform: %s", hostInfo.Platform)
	log.Infof("running version: %s", versionString(", "))

	// Tagger must be initialized after agent config has been setup
	tagger.Init()
	defer tagger.Stop() //nolint:errcheck

	err = initInfo(cfg)
	if err != nil {
		log.Criticalf("Error initializing info: %s", err)
		cleanupAndExit(1)
	}
	if err := statsd.Configure(cfg); err != nil {
		log.Criticalf("Error configuring statsd: %s", err)
		cleanupAndExit(1)
	}

	// Initialize system-probe heartbeats
	sysprobeMonitor, err := heartbeat.NewModuleMonitor(heartbeat.Options{
		KeysPerDomain:      api.KeysPerDomains(cfg.APIEndpoints),
		SysprobeSocketPath: cfg.SystemProbeAddress,
		HostName:           cfg.HostName,
		TagVersion:         Version,
		TagRevision:        GitCommit,
	})
	defer sysprobeMonitor.Stop()

	if err != nil {
		log.Warnf("failed to initialize system-probe monitor: %s", err)
	} else {
		sysprobeMonitor.Every(15 * time.Second)
	}

	// Exit if agent is not enabled and we're not debugging a check.
	if !cfg.Enabled && opts.check == "" {
		log.Infof(agent6DisabledMessage)

		// a sleep is necessary to ensure that supervisor registers this process as "STARTED"
		// If the exit is "too quick", we enter a BACKOFF->FATAL loop even though this is an expected exit
		// http://supervisord.org/subprocess.html#process-states
		time.Sleep(5 * time.Second)
		return
	}

	// update docker socket path in info
	dockerSock, err := util.GetDockerSocketPath()
	if err != nil {
		log.Debugf("Docker is not available on this host")
	}
	// we shouldn't quit because docker is not required. If no docker docket is available,
	// we just pass down empty string
	updateDockerSocket(dockerSock)

	if cfg.ProfilingEnabled {
		if err := enableProfiling(cfg); err != nil {
			log.Warnf("failed to enable profiling: %s", err)
		} else {
			log.Info("start profiling process-agent")
		}
		defer profiling.Stop()
	}

	log.Debug("Running process-agent with DEBUG logging enabled")
	if opts.check != "" {
		err := debugCheckResults(cfg, opts.check)
		if err != nil {
			fmt.Println(err)
			cleanupAndExit(1)
		} else {
			cleanupAndExit(0)
		}
		return
	}

	if opts.info {
		// using the debug port to get info to work
		url := fmt.Sprintf("http://localhost:%d/debug/vars", cfg.ProcessExpVarPort)
		if err := Info(os.Stdout, cfg, url); err != nil {
			cleanupAndExit(1)
		}
		return
	}

	// Run a profile & telemetry server.
	go func() {
		if ddconfig.Datadog.GetBool("telemetry.enabled") {
			http.Handle("/telemetry", telemetry.Handler())
		}
		err := http.ListenAndServe(fmt.Sprintf("localhost:%d", cfg.ProcessExpVarPort), nil)
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("Error creating expvar server on port %v: %v", cfg.ProcessExpVarPort, err)
		}
	}()

	cl, err := NewCollector(cfg)
	if err != nil {
		log.Criticalf("Error creating collector: %s", err)
		cleanupAndExit(1)
		return
	}
	if err := cl.run(exit); err != nil {
		log.Criticalf("Error starting collector: %s", err)
		os.Exit(1)
		return
	}

	for range exit {

	}
}

func debugCheckResults(cfg *config.AgentConfig, check string) error {
	sysInfo, err := checks.CollectSystemInfo(cfg)
	if err != nil {
		return err
	}

	if check == checks.Connections.Name() {
		// Connections check requires process-check to have occurred first (for process creation ts)
		checks.Process.Init(cfg, sysInfo)
		checks.Process.Run(cfg, 0) //nolint:errcheck
	}

	names := make([]string, 0, len(checks.All))
	for _, ch := range checks.All {
		if ch.Name() == check {
			ch.Init(cfg, sysInfo)
			return printResults(cfg, ch)
		}
		names = append(names, ch.Name())
	}
	return fmt.Errorf("invalid check '%s', choose from: %v", check, names)
}

func printResults(cfg *config.AgentConfig, ch checks.Check) error {
	// Run the check once to prime the cache.
	if _, err := ch.Run(cfg, 0); err != nil {
		return fmt.Errorf("collection error: %s", err)
	}

	time.Sleep(1 * time.Second)

	fmt.Printf("-----------------------------\n\n")
	fmt.Printf("\nResults for check %s\n", ch.Name())
	fmt.Printf("-----------------------------\n\n")

	msgs, err := ch.Run(cfg, 1)
	if err != nil {
		return fmt.Errorf("collection error: %s", err)
	}

	for _, m := range msgs {
		b, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal error: %s", err)
		}
		fmt.Println(string(b))
	}
	return nil
}

// cleanupAndExit cleans all resources allocated by the agent before calling
// os.Exit
func cleanupAndExit(status int) {
	// remove pidfile if set
	if opts.pidfilePath != "" {
		if _, err := os.Stat(opts.pidfilePath); err == nil {
			os.Remove(opts.pidfilePath)
		}
	}

	os.Exit(status)
}

func enableProfiling(cfg *config.AgentConfig) error {
	// allow full url override for development use
	s := ddconfig.DefaultSite
	if cfg.ProfilingSite != "" {
		s = cfg.ProfilingSite
	}

	site := fmt.Sprintf(profiling.ProfileURLTemplate, s)
	if cfg.ProfilingURL != "" {
		site = cfg.ProfilingURL
	}

	v, _ := version.Agent()

	return profiling.Start(cfg.ProfilingAPIKey, site, cfg.ProfilingEnvironment, "process-agent", fmt.Sprintf("version:%v", v))
}
