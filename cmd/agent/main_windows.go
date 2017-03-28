package main

import (
	_ "expvar"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
)

func main() {
	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		fmt.Printf("failed to determine if we are running in an interactive session: %v", err)
	}
	if !isIntSess {
		runService(false)
		return
	}
	// go_expvar server
	go http.ListenAndServe("127.0.0.1:5000", http.DefaultServeMux)

	// Invoke the Agent
	if err := app.AgentCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

var elog debug.Log

type myservice struct{}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
	statsd, collector, forwarder := app.StartAgent()
loop:
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
				app.StopAgent(statsd, collector, forwarder)
				break loop
			default:
				elog.Error(1, fmt.Sprintf("unexpected control request #%d", c))
			}
		case <-common.Stopper:
			elog.Info(1, "Received stop command, shutting down...")
			app.StopAgent(statsd, collector, forwarder)
			break loop

		}
	}
	changes <- svc.Status{State: svc.StopPending}
	return
}

func runService(isDebug bool) {
	var err error
	if isDebug {
		elog = debug.New(app.ServiceName)
	} else {
		elog, err = eventlog.Open(app.ServiceName)
		if err != nil {
			return
		}
	}
	defer elog.Close()

	elog.Info(1, fmt.Sprintf("starting %s service", app.ServiceName))
	run := svc.Run
	if isDebug {
		run = debug.Run
	}
	err = run(app.ServiceName, &myservice{})
	if err != nil {
		elog.Error(1, fmt.Sprintf("%s service failed: %v", app.ServiceName, err))
		return
	}
	elog.Info(1, fmt.Sprintf("%s service stopped", app.ServiceName))
}
