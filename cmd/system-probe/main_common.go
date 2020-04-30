package main

import (
	"bytes"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/system-probe/module"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/grpc"

	_ "net/http/pprof"
)

// Flag values
var opts struct {
	configPath  string
	pidFilePath string
	debug       bool
	version     bool
	checkCmd    *flag.FlagSet
	checkType   string
	checkClient string
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

func runAgent() {
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

	// Check if socket is available on unix, or Pipe is available on windows
	runCheck(cfg)

	log.Infof("running system-probe with version: %s", versionString(", "))

	// configure statsd
	if err := statsd.Configure(cfg); err != nil {
		log.Criticalf("Error configuring statsd: %s", err)
		cleanupAndExit(1)
	}

	sysprobe, err := CreateSystemProbe(cfg)
	if err != nil && strings.HasPrefix(err.Error(), ErrSysprobeUnsupported.Error()) {
		// If tracer is unsupported by this operating system, then exit gracefully
		log.Infof("%s, exiting.", err)
		gracefulExit()
	} else if err != nil {
		log.Criticalf("failed to create system probe: %s", err)
		cleanupAndExit(1)
	}
	defer sysprobe.Close()

	// WIP(safchain) modules draft
	lis, err := net.Listen("tcp", ":8787")
	if err != nil {
		log.Criticalf("failed to create system probe: %s", err)
		os.Exit(1)
	}
	grpcServer := grpc.NewServer()

	specs := []module.Spec{
		module.Spec{
			Name: "security-module",
			New:  secmodule.NewModule,
		},
	}

	factory := module.NewFactory()
	if err := factory.Run(cfg, specs, module.Opts{GRPCServer: grpcServer}); err != nil {
		log.Criticalf("failed to instantiate modules: %s", err)
		os.Exit(1)
	}
	defer factory.Stop()

	go grpcServer.Serve(lis)
	// /WIP(safchain) modules draft

	go sysprobe.Run()
	log.Infof("system probe successfully started")

	// Handles signals, which tells us whether we should exit.
	e := make(chan bool)
	go util.HandleSignals(e)
	<-e
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
