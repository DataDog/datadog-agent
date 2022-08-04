// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package initcontainer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	serverlessLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/metric"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/serverless"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Run is the entrypoint of the init process. It will spawn the customer process
func Run(logConfig *serverlessLog.Config, metricAgent *metrics.ServerlessMetricAgent, traceAgent *trace.ServerlessTraceAgent, args []string) {
	serverlessLog.Write(logConfig, []byte(fmt.Sprintf("[datadog init process] running cmd = >%v<", args)))
	err := execute(logConfig, metricAgent, traceAgent, args)
	if err != nil {
		serverlessLog.Write(logConfig, []byte(fmt.Sprintf("[datadog init process] exiting with code = %s", err)))
	} else {
		serverlessLog.Write(logConfig, []byte("[datadog init process] exiting successfully"))
	}
}

func execute(config *serverlessLog.Config, metricAgent *metrics.ServerlessMetricAgent, traceAgent *trace.ServerlessTraceAgent, args []string) error {
	commandName, commandArgs := buildCommandParam(args)
	cmd := exec.Command(commandName, commandArgs...)
	cmd.Stdout = &serverlessLog.CustomWriter{
		LogConfig:  config,
		LineBuffer: bytes.Buffer{},
	}
	cmd.Stderr = &serverlessLog.CustomWriter{
		LogConfig:  config,
		LineBuffer: bytes.Buffer{},
	}
	handleSignals(cmd.Process, config, metricAgent, traceAgent)
	err := cmd.Start()
	if err != nil {
		return err
	}
	err = cmd.Wait()
	flush(config.FlushTimeout, metricAgent, traceAgent)
	return err
}

func buildCommandParam(cmdArg []string) (string, []string) {
	fields := cmdArg
	if len(cmdArg) == 1 {
		fields = strings.Fields(cmdArg[0])
	}
	commandName := fields[0]
	if len(fields) > 1 {
		return commandName, fields[1:]
	}
	return commandName, []string{}
}

func handleSignals(process *os.Process, config *serverlessLog.Config, metricAgent *metrics.ServerlessMetricAgent, traceAgent *trace.ServerlessTraceAgent) {
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs)
		for sig := range sigs {
			if sig != syscall.SIGURG {
				serverlessLog.Write(config, []byte(fmt.Sprintf("[datadog init process] %s received", sig)))
			}
			if sig != syscall.SIGCHLD {
				if process != nil {
					_ = syscall.Kill(process.Pid, sig.(syscall.Signal))
				}
			}
			if sig == syscall.SIGTERM {
				metric.AddShutdownMetric(metricAgent.GetExtraTags(), time.Now(), metricAgent.Demux)
				flush(config.FlushTimeout, metricAgent, traceAgent)
				os.Exit(0)
			}
		}
	}()
}

func flush(flushTimeout time.Duration, metricAgent serverless.FlushableAgent, traceAgent serverless.FlushableAgent) bool {
	var hasTimeout int32
	wg := &sync.WaitGroup{}
	wg.Add(3)
	go flushAndWait(flushTimeout, wg, metricAgent, hasTimeout)
	go flushAndWait(flushTimeout, wg, traceAgent, hasTimeout)
	childCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	go func(wg *sync.WaitGroup, ctx context.Context) {
		logs.Flush(ctx)
		wg.Done()
	}(wg, childCtx)
	wg.Wait()
	return atomic.LoadInt32(&hasTimeout) > 0
}

func flushWithContext(ctx context.Context, timeout time.Duration, timeoutchan chan struct{}, flushFunction func()) {
	flushFunction()
	select {
	case timeoutchan <- struct{}{}:
		log.Debug("finished flushing")
	case <-ctx.Done():
		log.Error("timed out while flushing")
		return
	}
}

func flushAndWait(flushTimeout time.Duration, wg *sync.WaitGroup, agent serverless.FlushableAgent, hasTimeout int32) {
	childCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	ch := make(chan struct{}, 1)
	go flushWithContext(childCtx, flushTimeout, ch, agent.Flush)
	select {
	case <-childCtx.Done():
		atomic.AddInt32(&hasTimeout, 1)
		break
	case <-ch:
		break
	}
	wg.Done()
}
