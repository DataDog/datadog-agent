// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

//lint:file-ignore U1000 Trace Pipe Logger is useful for debugging

package tests

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//nolint:unused
func (tm *testModule) startTracing() (*tracePipeLogger, error) {
	tracePipe, err := NewTracePipe()
	if err != nil {
		return nil, err
	}

	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}

	logger := &tracePipeLogger{
		TracePipe:  tracePipe,
		stop:       make(chan struct{}),
		executable: filepath.Base(executable),
	}
	logger.Start()

	time.Sleep(time.Millisecond * 200)

	return logger, nil
}

//nolint:unused
type tracePipeLogger struct {
	*TracePipe
	stop       chan struct{}
	executable string
}

//nolint:unused
func (l *tracePipeLogger) handleEvent(event *TraceEvent) {
	// for some reason, the event task is resolved to "<...>"
	// so we check that event.PID is the ID of a task of the running process
	taskPath := filepath.Join(util.HostProc(), strconv.Itoa(int(utils.Getpid())), "task", event.PID)
	_, err := os.Stat(taskPath)

	if event.Task == l.executable || (event.Task == "<...>" && err == nil) {
		log.Debug(event.Raw)
	}
}

//nolint:unused
func (l *tracePipeLogger) Start() {
	channelEvents, channelErrors := l.Channel()

	go func() {
		for {
			select {
			case <-l.stop:
				for len(channelEvents) > 0 {
					l.handleEvent(<-channelEvents)
				}
				return
			case event := <-channelEvents:
				l.handleEvent(event)
			case err := <-channelErrors:
				log.Error(err)
			}
		}
	}()
}

//nolint:unused
func (l *tracePipeLogger) Stop() {
	time.Sleep(time.Millisecond * 200)

	l.stop <- struct{}{}
	l.Close()
}
