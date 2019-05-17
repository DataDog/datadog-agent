package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	_ "net/http/pprof"
)

// Flag values
var opts struct {
	configPath string

	pidFilePath string
	debug       bool
	version     bool
}

// Version info sourced from build flags
var (
	GoVersion string
	Version   string
	GitCommit string
	GitBranch string
	BuildDate string
)

const loggerName = ddconfig.LoggerName("SYSPROBE")

func main() {
	// Parse flags
	flag.StringVar(&opts.configPath, "config", "/etc/datadog-agent/system-probe.yaml", "Path to system-probe config formatted as YAML")
	flag.StringVar(&opts.pidFilePath, "pid", "", "Path to set pidfile for process")
	flag.BoolVar(&opts.version, "version", false, "Print the version and exit")
	flag.Parse()

	// Set up a default config before parsing config so we log errors nicely.
	// The default will be stdout since we can't assume any file is writable.
	if err := config.SetupInitialLogger(loggerName); err != nil {
		panic(err)
	}
	defer log.Flush()

	// --version
	if opts.version {
		fmt.Println(versionString("\n"))
		os.Exit(0)
	}

	// --pid
	if opts.pidFilePath != "" {
		if err := pidfile.WritePID(opts.pidFilePath); err != nil {
			log.Errorf("Error while writing PID file, exiting: %v", err)
			os.Exit(1)
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
		os.Exit(1)
	}

	// Exit if system probe is disabled
	if !cfg.EnableSystemProbe {
		log.Info("system probe not enabled. exiting.")
		gracefulExit()
	}

	sysprobe, err := CreateSystemProbe(cfg)
	if err != nil && strings.HasPrefix(err.Error(), ErrTracerUnsupported.Error()) {
		// If tracer is unsupported by this operating system, then exit gracefully
		log.Infof("%s, exiting.", err)
		gracefulExit()
	} else if err != nil {
		log.Criticalf("failed to create system probe: %s", err)
		os.Exit(1)
	}
	defer sysprobe.Close()

	platform, err := util.GetPlatform()
	if err != nil {
		log.Debugf("error retrieving platform: %s", err)
	} else {
		log.Infof("running on platform: %s", platform)
	}
	log.Infof("running system-probe with version: %s", versionString(", "))
	go sysprobe.Run()
	log.Infof("system probe started")

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
	os.Exit(0)
}

func handleSignals(exit chan bool) {
	sigIn := make(chan os.Signal, 100)
	signal.Notify(sigIn)
	// unix only in all likelihood;  but we don't care.
	for sig := range sigIn {
		switch sig {
		case syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGQUIT:
			log.Criticalf("Caught signal '%s'; terminating.", sig)
			close(exit)
		default:
			log.Warnf("Caught signal %s; continuing/ignoring.", sig)
		}
	}
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
