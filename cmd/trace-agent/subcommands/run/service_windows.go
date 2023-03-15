// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/trace/config"
	tracecfg "github.com/DataDog/datadog-agent/pkg/trace/config"

	"golang.org/x/sys/windows/svc"
)

type myservice struct {
	cliParams *RunParams
	config    config.Component
}

func (m *myservice) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	ctx, cancelFunc := context.WithCancel(context.Background())

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
				case svc.Stop, svc.PreShutdown, svc.Shutdown:
					elog.Info(0x40000006, tracecfg.ServiceName)
					changes <- svc.Status{State: svc.StopPending}
					cancelFunc()
					return
				default:
					elog.Warning(0xc000000A, fmt.Sprint(c.Cmd))
				}
			}
		}
	}()
	elog.Info(0x40000003, tracecfg.ServiceName)
	runAgent(ctx, m.cliParams, m.config)

	changes <- svc.Status{State: svc.Stopped}
	return
}
