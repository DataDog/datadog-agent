// +build linux windows

package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	_ "net/http/pprof"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// All System Probe modules should register their factories here
var factories = []api.Factory{
	modules.NetworkTracer,
	modules.TCPQueueLength,
	modules.OOMKillProbe,
}

// Flag values
var opts struct {
	configPath  string
	pidFilePath string
	debug       bool
	version     bool
	console     bool // windows only; execute on console rather than via SCM
}

// Version info sourced from build flags
var (
	GoVersion string
	Version   string
	GitCommit string
	GitBranch string
	BuildDate string
)

const loggerName = ddconfig.LoggerName("SYS-PROBE")

func runAgent(exit <-chan struct{}) {
	// --version
	if opts.version {
		fmt.Println(versionString("\n"))
		cleanupAndExit(0)
	}

	// --pid
	if opts.pidFilePath != "" {
		if err := pidfile.WritePID(opts.pidFilePath); err != nil {
			log.Errorf("Error while writing PID file, exiting: %v", err)
			cleanupAndExit(1)
		}

		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), opts.pidFilePath)

		defer func() {
			os.Remove(opts.pidFilePath)
		}()
	}

	// Parsing YAML config files
	cfg, err := config.NewSystemProbeConfig(loggerName, opts.configPath)
	if err != nil {
		log.Criticalf("Failed to create agent config: %s", err)
		cleanupAndExit(1)
	}

	// Exit if system probe is disabled
	if !cfg.EnableSystemProbe {
		log.Info("system probe not enabled. exiting.")
		gracefulExit()
	}

	log.Infof("running system-probe with version: %s", versionString(", "))

	// configure statsd
	if err := statsd.Configure(cfg); err != nil {
		log.Criticalf("Error configuring statsd: %s", err)
		cleanupAndExit(1)
	}

	conn, err := net.NewListener(cfg)
	if err != nil {
		log.Criticalf("Error creating IPC socket: %s", err)
		cleanupAndExit(1)
	}

	loader := NewLoader()
	httpMux := http.NewServeMux()

	err = loader.Register(cfg, httpMux, factories)
	if err != nil && strings.HasPrefix(err.Error(), modules.ErrSysprobeUnsupported.Error()) {
		// If tracer is unsupported by this operating system, then exit gracefully
		log.Infof("%s, exiting.", err)
		gracefulExit()
	}
	if err != nil {
		log.Criticalf("failed to create system probe: %s", err)
		cleanupAndExit(1)
	}
	defer loader.Close()

	// Register stats endpoint
	httpMux.HandleFunc("/debug/stats", func(w http.ResponseWriter, req *http.Request) {
		stats := loader.GetStats()
		utils.WriteAsJSON(w, stats)
	})

	go func() {
		err = http.Serve(conn.GetListener(), httpMux)
		if err != nil {
			log.Criticalf("Error creating HTTP server: %s", err)
			cleanupAndExit(1)
		}
	}()

	log.Infof("system probe successfully started")

	go func() {
		tags := []string{
			fmt.Sprintf("version:%s", Version),
			fmt.Sprintf("revision:%s", GitCommit),
		}
		heartbeat := time.NewTicker(15 * time.Second)
		for range heartbeat.C {
			statsd.Client.Gauge("datadog.system_probe.agent", 1, tags, 1) //nolint:errcheck
		}
	}()

	<-exit
}

func gracefulExit() {
	// A sleep is necessary to ensure that supervisor registers this process as "STARTED"
	// If the exit is "too quick", we enter a BACKOFF->FATAL loop even though this is an expected exit
	// http://supervisord.org/subprocess.html#process-states
	time.Sleep(5 * time.Second)
	cleanupAndExit(0)
}

// versionString returns the version information filled in at build time
func versionString(sep string) string {
	addString := func(buf *bytes.Buffer, s, arg string, sep string) {
		if arg != "" {
			fmt.Fprintf(buf, s, arg, sep)
		}
	}

	var buf bytes.Buffer
	addString(&buf, "Version: %s%s", Version, sep)
	addString(&buf, "Git hash: %s%s", GitCommit, sep)
	addString(&buf, "Git branch: %s%s", GitBranch, sep)
	addString(&buf, "Build date: %s%s", BuildDate, sep)
	addString(&buf, "Go Version: %s%s", GoVersion, sep)
	return buf.String()
}

// cleanupAndExit cleans all resources allocated by system-probe before calling
// os.Exit
func cleanupAndExit(status int) {
	// remove pidfile if set
	if opts.pidFilePath != "" {
		if _, err := os.Stat(opts.pidFilePath); err == nil {
			os.Remove(opts.pidFilePath)
		}
	}

	os.Exit(status)
}
