// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package initcontainer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/log"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/metric"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/timing"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/serverless"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
)

func Run(logConfig *log.Config, metricAgent *metrics.ServerlessMetricAgent, traceAgent *trace.ServerlessTraceAgent, args []string) {
	log.Write(logConfig, []byte(fmt.Sprintf("[datadog init process] running cmd = >%v<", args)))
	err := execute(logConfig, metricAgent, traceAgent, args)
	if err != nil {
		log.Write(logConfig, []byte(fmt.Sprintf("[datadog init process] exiting with code = %s", err)))
	} else {
		log.Write(logConfig, []byte("[datadog init process] exiting successfully"))
	}
}

func execute(config *log.Config, metricAgent *metrics.ServerlessMetricAgent, traceAgent *trace.ServerlessTraceAgent, args []string) error {
	commandName, commandArgs := buildCommandParam(args)
	cmd := exec.Command(commandName, commandArgs...)
	cmd.Stdout = &log.CustomWriter{
		LogConfig: config,
	}
	cmd.Stderr = &log.CustomWriter{
		LogConfig: config,
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

func handleSignals(process *os.Process, config *log.Config, metricAgent *metrics.ServerlessMetricAgent, traceAgent *trace.ServerlessTraceAgent) {
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs)
		for sig := range sigs {
			if sig != syscall.SIGURG {
				log.Write(config, []byte(fmt.Sprintf("[datadog init process] %s received", sig)))
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
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()

	wg := &sync.WaitGroup{}
	wg.Add(3)

	go func(wg *sync.WaitGroup) {
		metricAgent.Flush()
		wg.Done()
	}(wg)

	go func(wg *sync.WaitGroup) {
		traceAgent.Flush()
		wg.Done()
	}(wg)

	go func(wg *sync.WaitGroup, ctx context.Context) {
		logs.Flush(ctx)
		wg.Done()
	}(wg, ctx)

	return timing.WaitWithTimeout(wg, flushTimeout)
}
