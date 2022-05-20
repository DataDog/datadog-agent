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
	"github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	"github.com/DataDog/datadog-agent/pkg/logs"
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
	commandName, commandArgs := buildCommandParam(args[0])
	cmd := exec.Command(commandName, commandArgs...)
	cmd.Stdout = &log.CustomWriter{
		LogConfig: config,
	}
	cmd.Stderr = &log.CustomWriter{
		LogConfig: config,
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	handleSignals(cmd.Process, config, metricAgent)
	err := cmd.Start()
	if err != nil {
		return err
	}
	err = cmd.Wait()
	flush(config, metricAgent, traceAgent)
	return err
}

func buildCommandParam(cmdArg string) (string, []string) {
	fields := strings.Fields(cmdArg)
	commandName := fields[0]
	if len(fields) > 1 {
		return commandName, fields[1:]
	}
	return commandName, []string{}
}

func handleSignals(process *os.Process, config *log.Config, metricAgent *metrics.ServerlessMetricAgent) {
	sigs := make(chan os.Signal, 1)
	defer close(sigs)
	signal.Notify(sigs)
	defer signal.Reset()
	go func() {
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
				metric.AddShutdownMetric(tag.GetBaseTags(), time.Now(), metricAgent.Demux)
			}
		}
	}()
}

func flush(config *log.Config, metricAgent *metrics.ServerlessMetricAgent, traceAgent *trace.ServerlessTraceAgent) {
	ctx, cancel := context.WithTimeout(context.Background(), config.FlushTimeout)
	defer cancel()

	wg := &sync.WaitGroup{}
	wg.Add(3)

	go func(wg *sync.WaitGroup) {
		metricAgent.Flush()
		wg.Done()
	}(wg)

	go func(wg *sync.WaitGroup) {
		traceAgent.Get().FlushSync()
		wg.Done()
	}(wg)

	go func(wg *sync.WaitGroup, ctx context.Context) {
		logs.Flush(ctx)
		wg.Done()
	}(wg, ctx)

	waitWithTimeout(wg, config.FlushTimeout)
}

func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return
	case <-time.After(timeout):
		return
	}
}
