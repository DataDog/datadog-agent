// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package service provides a way to interact with os services
package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/containerd/fifo"
)

type unitCommand string

const (
	startUnit     = unitCommand("start")
	stopUnit      = unitCommand("stop")
	enableUnit    = unitCommand("enable")
	disableUnit   = unitCommand("disable")
	loadUnit      = unitCommand("load-unit")
	removeUnit    = unitCommand("remove-systemd")
	systemdReload = "systemd-reload"
	adminExecutor = "datadog-updater-admin.service"
)

var (
	inFifoPath  = filepath.Join(setup.InstallPath, "run", "in.fifo")
	outFifoPath = filepath.Join(setup.InstallPath, "run", "out.fifo")
	runnerMutex sync.Mutex
)

type scriptRunner struct {
	inFifo  io.ReadWriteCloser
	outFifo io.ReadWriteCloser
	timeout time.Duration
}

func newScriptRunner() (*scriptRunner, error) {
	runnerMutex.Lock()
	err := os.Remove(inFifoPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("error deleting inFifo: %s", err)
	}
	err = os.Remove(outFifoPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("error deleting inFifo: %s", err)
	}

	// start with outFifo creation first as inFifo triggers admin exec systemd path listner
	outFifo, err := fifo.OpenFifo(context.Background(), outFifoPath, syscall.O_CREAT|syscall.O_RDONLY|syscall.O_NONBLOCK, 0660)
	if err != nil {
		return nil, fmt.Errorf("error opening out.fifo: %s", err)
	}

	// creating a inFifo triggers admin exec systemd path listner
	// If a previous runner is stuck on write due to a crash of the updater
	// it gets killed and a new updater is spawned
	inFifo, err := fifo.OpenFifo(context.Background(), inFifoPath, syscall.O_CREAT|syscall.O_WRONLY|syscall.O_NONBLOCK, 0660)
	if err != nil {
		outFifo.Close()
		return nil, fmt.Errorf("error opening in.fifo: %s", err)
	}
	return &scriptRunner{
		inFifo:  inFifo,
		outFifo: outFifo,
		timeout: 10 * time.Second,
	}, nil
}

func (s *scriptRunner) stopUnit(unit string) error {
	return s.executeCommand(wrapUnitCommand(stopUnit, unit))
}

func (s *scriptRunner) startUnit(unit string) error {
	return s.executeCommand(wrapUnitCommand(startUnit, unit))
}

func (s *scriptRunner) enableUnit(unit string) error {
	return s.executeCommand(wrapUnitCommand(enableUnit, unit))
}

func (s *scriptRunner) disableUnit(unit string) error {
	return s.executeCommand(wrapUnitCommand(disableUnit, unit))
}

func (s *scriptRunner) loadUnit(unit string) error {
	return s.executeCommand(wrapUnitCommand(loadUnit, unit))
}

func (s *scriptRunner) removeUnit(unit string) error {
	return s.executeCommand(wrapUnitCommand(removeUnit, unit))
}

func (s *scriptRunner) systemdReload() error {
	return s.executeCommand(systemdReload)
}

func (s *scriptRunner) executeCommand(command string) error {
	err := wrapWithTimeout(func() error {
		_, err := s.inFifo.Write([]byte(command + "\n"))
		return err
	},
		s.timeout,
	)
	if err != nil {
		return fmt.Errorf("error executing command %s while writing to fifo: %s", command, err)
	}
	bufReader := bufio.NewReader(s.outFifo)
	var res string
	err = wrapWithTimeout(func() error {
		res, err = bufReader.ReadString('\n')
		return err
	},
		s.timeout,
	)
	if err != nil {
		return fmt.Errorf("error executing command %s while reading from fifo: %s", command, err)
	}
	result := strings.TrimRight(string(res), "\n")
	if result != "success" {
		return fmt.Errorf("error executing command %s: %s", command, result)
	}
	return nil
}

func (s *scriptRunner) close() {
	if err := s.inFifo.Close(); err != nil {
		log.Warnf("error closing inFifo: %s", err)
	}
	if err := os.Remove(inFifoPath); err != nil {
		log.Warnf("error removing inFifo: %s", err)
	}
	if err := s.outFifo.Close(); err != nil {
		log.Warnf("error closing outFifo: %s", err)
	}
	if err := os.Remove(outFifoPath); err != nil {
		log.Warnf("error removing outFifo: %s", err)
	}
	runnerMutex.Unlock()
}

func wrapUnitCommand(command unitCommand, unit string) string {
	return string(command) + " " + unit
}

// wrapWithTimeout wraps calls with a timeout
// The timeout to avoid locking the updater if the admin-exec process
// gets killed.
// Relauching the systemd commands will trigger a new admin-exec process
func wrapWithTimeout(fn func() error, timeout time.Duration) error {
	err := make(chan error, 1)
	go func() {
		err <- fn()
	}()
	select {
	case <-time.After(timeout):
		return fmt.Errorf("timeout")
	case e := <-err:
		return e
	}
}
