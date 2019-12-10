package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	_ "net/http/pprof"
)

// Flag values
var opts struct {
	configPath  string
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

const loggerName = ddconfig.LoggerName("SYS-PROBE")

func main() {
	// Parse flags
	flag.StringVar(&opts.configPath, "config", "/etc/datadog-agent/system-probe.yaml", "Path to system-probe config formatted as YAML")
	flag.StringVar(&opts.pidFilePath, "pid", "", "Path to set pidfile for process")
	checkCmd := flag.NewFlagSet("check", flag.ExitOnError)
	checkType := checkCmd.String("type", "", "The type of the check to run. Choose from: connections, network_maps, network_state, stats")
	checkClient := checkCmd.String("client", "", "The client ID that the check will use to run")
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

	// run check command if the flag is specified
	if len(os.Args) >= 2 && os.Args[1] == "check" {
		err = checkCmd.Parse(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if *checkType == "" {
			checkCmd.PrintDefaults()
			os.Exit(1)
		}
		err := querySocketEndpoint(cfg, *checkType, *checkClient)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		os.Exit(0)
	}

	log.Infof("running system-probe with version: %s", versionString(", "))

	// configure statsd
	if err := statsd.Configure(cfg); err != nil {
		log.Criticalf("Error configuring statsd: %s", err)
		os.Exit(1)
	}

	sysprobe, err := CreateSystemProbe(cfg)
	if err != nil && strings.HasPrefix(err.Error(), ErrSysprobeUnsupported.Error()) {
		// If tracer is unsupported by this operating system, then exit gracefully
		log.Infof("%s, exiting.", err)
		gracefulExit()
	} else if err != nil {
		log.Criticalf("failed to create system probe: %s", err)
		os.Exit(1)
	}
	defer sysprobe.Close()

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
