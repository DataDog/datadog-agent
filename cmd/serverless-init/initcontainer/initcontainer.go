// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

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
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/metric"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	serverlessLog "github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/pkg/serverless"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/afero"
)

// Run is the entrypoint of the init process. It will spawn the customer process
func Run(
	cloudService cloudservice.CloudService,
	logConfig *serverlessLog.Config,
	metricAgent *metrics.ServerlessMetricAgent,
	traceAgent *trace.ServerlessTraceAgent,
	logsAgent logsAgent.ServerlessLogsAgent,
	args []string,
) {
	serverlessLog.Write(logConfig, []byte(fmt.Sprintf("[datadog init process] running cmd = >%v<", args)), false)
	err := execute(logConfig, args)
	if err != nil {
		serverlessLog.Write(logConfig, []byte(fmt.Sprintf("[datadog init process] exiting with code = %s", err)), false)
	} else {
		serverlessLog.Write(logConfig, []byte("[datadog init process] exiting successfully"), false)
	}
	metric.AddShutdownMetric(cloudService.GetPrefix(), metricAgent.GetExtraTags(), time.Now(), metricAgent.Demux)
	flush(logConfig.FlushTimeout, metricAgent, traceAgent, logsAgent)
}

func execute(logConfig *serverlessLog.Config, args []string) error {
	commandName, commandArgs := buildCommandParam(args)

	// Add our tracer settings
	fs := afero.NewOsFs()
	AutoInstrumentTracer(fs)

	cmd := exec.Command(commandName, commandArgs...)

	shouldBuffer := calculateShouldBuffer(commandName)

	cmd.Stdout = &serverlessLog.CustomWriter{
		LogConfig:  logConfig,
		LineBuffer: bytes.Buffer{},
		// Dotnet occasionally writes to stdout in multiple chunks causing log splitting issues.
		// This happens regardless of logging library (and happens with Console.WriteLine).
		// ShouldBuffer tells the CustomWriter to buffer all log chunks that don't end in a newline,
		// fixing log splitting in this scenario.
		ShouldBuffer: shouldBuffer,
	}
	cmd.Stderr = &serverlessLog.CustomWriter{
		LogConfig:    logConfig,
		LineBuffer:   bytes.Buffer{},
		ShouldBuffer: shouldBuffer,
		IsError:      true,
	}
	err := cmd.Start()
	if err != nil {
		return err
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs)
	go forwardSignals(cmd.Process, logConfig, sigs)
	err = cmd.Wait()
	return err
}

func calculateShouldBuffer(commandName string) bool {
	return commandName == "dotnet"
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

func forwardSignals(process *os.Process, config *serverlessLog.Config, sigs chan os.Signal) {
	for sig := range sigs {
		if sig != syscall.SIGURG {
			serverlessLog.Write(config, []byte(fmt.Sprintf("[datadog init process] %s received", sig)), false)
		}
		if sig != syscall.SIGCHLD {
			if process != nil {
				_ = syscall.Kill(process.Pid, sig.(syscall.Signal))
			}
		}
	}
}

func flush(flushTimeout time.Duration, metricAgent serverless.FlushableAgent, traceAgent serverless.FlushableAgent, logsAgent logsAgent.ServerlessLogsAgent) bool {
	hasTimeout := atomic.NewInt32(0)
	wg := &sync.WaitGroup{}
	wg.Add(3)
	go flushAndWait(flushTimeout, wg, metricAgent, hasTimeout)
	go flushAndWait(flushTimeout, wg, traceAgent, hasTimeout)
	childCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	go func(wg *sync.WaitGroup, ctx context.Context) {
		if logsAgent != nil {
			logsAgent.Flush(ctx)
		}
		wg.Done()
	}(wg, childCtx)
	wg.Wait()
	return hasTimeout.Load() > 0
}

func flushWithContext(ctx context.Context, timeoutchan chan struct{}, flushFunction func()) {
	flushFunction()
	select {
	case timeoutchan <- struct{}{}:
		log.Debug("finished flushing")
	case <-ctx.Done():
		log.Error("timed out while flushing")
		return
	}
}

func flushAndWait(flushTimeout time.Duration, wg *sync.WaitGroup, agent serverless.FlushableAgent, hasTimeout *atomic.Int32) {
	childCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	ch := make(chan struct{}, 1)
	go flushWithContext(childCtx, ch, agent.Flush)
	select {
	case <-childCtx.Done():
		hasTimeout.Inc()
		break
	case <-ch:
		break
	}
	wg.Done()
}
