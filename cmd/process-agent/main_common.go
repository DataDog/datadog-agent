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
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const loggerName ddconfig.LoggerName = "PROCESS"

var opts struct {
	configPath    string
	netConfigPath string
	pidfilePath   string
	debug         bool
	version       bool
	check         string
	info          bool
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

func runAgent(exit chan bool) {
	platform, err := util.GetPlatform()
	if err != nil {
		log.Debugf("error retrieving platform: %s", err)
	} else {
		log.Infof("running on platform: %s", platform)
	}

	if opts.version {
		fmt.Print(versionString("\n"))
		os.Exit(0)
	}
	log.Infof("running version: %s", versionString(", "))

	if opts.check == "" && !opts.info && opts.pidfilePath != "" {
		err := pidfile.WritePID(opts.pidfilePath)
		if err != nil {
			log.Errorf("Error while writing PID file, exiting: %v", err)
			os.Exit(1)
		}

		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), opts.pidfilePath)
		defer func() {
			// remove pidfile if set
			os.Remove(opts.pidfilePath)
		}()
	}

	cfg, err := config.NewAgentConfig(loggerName, opts.configPath, opts.netConfigPath)
	if err != nil {
		log.Criticalf("Error parsing config: %s", err)
		os.Exit(1)
	}

	// Tagger must be initialized after agent config has been setup
	tagger.Init()
	defer tagger.Stop()

	err = initInfo(cfg)
	if err != nil {
		log.Criticalf("Error initializing info: %s", err)
		os.Exit(1)
	}
	if err := statsd.Configure(cfg); err != nil {
		log.Criticalf("Error configuring statsd: %s", err)
		os.Exit(1)
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

	log.Debug("Running process-agent with DEBUG logging enabled")
	if opts.check != "" {
		err := debugCheckResults(cfg, opts.check)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		} else {
			os.Exit(0)
		}
		return
	}

	if opts.info {
		// using the debug port to get info to work
		url := fmt.Sprintf("http://localhost:%d/debug/vars", cfg.ProcessExpVarPort)
		if err := Info(os.Stdout, cfg, url); err != nil {
			os.Exit(1)
		}
		return
	}

	// Run a profile server.
	go func() {
		http.ListenAndServe(fmt.Sprintf("localhost:%d", cfg.ProcessExpVarPort), nil)
	}()

	cl, err := NewCollector(cfg)
	if err != nil {
		log.Criticalf("Error creating collector: %s", err)
		os.Exit(1)
		return
	}
	cl.run(exit)
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
		checks.Process.Run(cfg, 0)
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

	if cfg.EnableLocalNetworkTracer && ch.Name() == checks.Connections.Name() {
		fmt.Printf("Waiting 5 seconds to allow for active connections to transmit data\n")
		time.Sleep(5 * time.Second)
	} else {
		time.Sleep(1 * time.Second)
	}

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
