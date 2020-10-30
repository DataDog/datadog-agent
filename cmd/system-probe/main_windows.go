// +build windows

package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
)

var elog debug.Log

// ServiceName is the service name used for the process-agent
const ServiceName = "datadog-system-probe"

// opts are the command-line options
var defaultAgentConfigPath = "c:\\programdata\\datadog\\datadog.yaml"
var defaultSysProbeConfigPath = "c:\\programdata\\datadog\\system-probe.yaml"

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultAgentConfigPath = filepath.Join(pd, "datadog.yaml")
		defaultSysProbeConfigPath = filepath.Join(pd, "system-probe.yaml")
	}
}

func main() {
	flag.StringVar(&opts.agentConfigPath, "agent-config", defaultAgentConfigPath, "Path to agent config formatted as YAML")
	flag.StringVar(&opts.configPath, "config", defaultSysProbeConfigPath, "Path to system-probe config formatted as YAML")
	flag.StringVar(&opts.pidFilePath, "pid", "", "Path to set pidfile for process")
	flag.BoolVar(&opts.version, "version", false, "Print the version and exit")
	flag.BoolVar(&opts.console, "console", false, "Run as console application rather than service")
	flag.Parse()

	if !opts.console {
		isIntSess, err := svc.IsAnInteractiveSession()
		if err != nil {
			fmt.Printf("Failed to determine if we are running in an interactive session: %v\n", err)
		}
		if !isIntSess {
			runService(false)
			return
		}
	}
	// Handles signals, which tells us whether we should exit.
	exit := make(chan struct{})
	go util.HandleSignals(exit)
	runAgent(exit)

}

func runCheck(cfg *config.AgentConfig) {
	return
}

type myservice struct{}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	exit := make(chan struct{})

	go func() {
		for {
			select {
			case c := <-r:
				switch c.Cmd {
				case svc.Interrogate:
					changes <- c.CurrentStatus
					// Testing deadlock from https://code.google.com/p/winsvc/issues/detail?id=4
					time.Sleep(100 * time.Millisecond)
					changes <- c.CurrentStatus
				case svc.Stop, svc.Shutdown:
					elog.Info(0x40000006, ServiceName)
					changes <- svc.Status{State: svc.StopPending}
					///// FIXME:  Need a way to indicate to rest of service to shut
					////  down
					close(exit)
					break
				default:
					elog.Warning(0xc000000A, string(c.Cmd))
				}
			}
		}
	}()
	elog.Info(0x40000003, ServiceName)
	runAgent(exit)

	changes <- svc.Status{State: svc.Stopped}
	return
}

func runService(isDebug bool) {
	var err error
	if isDebug {
		elog = debug.New(ServiceName)
	} else {
		elog, err = eventlog.Open(ServiceName)
		if err != nil {
			return
		}
	}
	defer elog.Close()

	run := svc.Run
	if isDebug {
		run = debug.Run
	}
	elog.Info(0x40000007, ServiceName)
	err = run(ServiceName, &myservice{})
	if err != nil {
		elog.Error(0xc0000008, err.Error())
		return
	}
	elog.Info(0x40000004, ServiceName)
}
