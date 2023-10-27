// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.
//go:build windows

// Package apmetwtracerimpl provides a component for the .Net tracer application
package apmetwtracerimpl

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/apm/etwtracer"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/etw"
	"github.com/Microsoft/go-winio"
	"go.uber.org/fx"
	"golang.org/x/sys/windows"
	"io"
	"net"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	namedPipePath = "\\\\.\\pipe\\DD_ETW_DISPATCHER"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newApmEtwTracerImpl),
)

type dependencies struct {
	fx.In
	Lc  fx.Lifecycle
	Log log.Component
	etw etw.Component
}

func newApmEtwTracerImpl(deps dependencies) (apmetwtracer.Component, error) {
	// Microsoft-Windows-DotNETRuntime
	guid, _ := windows.GUIDFromString("{E13C0D23-CCBC-4E12-931B-D9CC2EEE27E4}")

	apmEtwTracer := &apmetwtracerimpl{
		log:                       deps.Log,
		etw:                       deps.etw,
		dotNetRuntimeProviderGuid: guid,
	}

	deps.Lc.Append(fx.Hook{OnStart: apmEtwTracer.start, OnStop: apmEtwTracer.stop})
	return apmEtwTracer, nil
}

type apmetwtracerimpl struct {
	session                   etw.Session
	dotNetRuntimeProviderGuid windows.GUID

	// PIDs contains the list of PIDs we are interested in
	// We use a map it's more appropriate for frequent add / remove and because struct{} occupies 0 bytes
	pids map[uint64]struct{}

	pipeListener net.Listener
	log          log.Component
	etw          etw.Component
}

type header struct {
	magic           []byte
	size            uint16
	commandResponse uint8
}

func (a *apmetwtracerimpl) handleConnection(c net.Conn) {
	defer c.Close()
	a.log.Debugf("client connected [%s]", c.RemoteAddr().Network())

	buf := make([]byte, 512)
	for {
		n, err := c.Read(buf)
		if err != nil {
			if err != io.EOF {
				a.log.Debugf("read error: %v\n", err)
			}
			break
		}
		p := (*header)(unsafe.Pointer(unsafe.SliceData(buf[:n])))
		a.log.Debugf("received message: %s with magic %s", p.commandResponse, string(p.magic))
	}
	a.log.Debugf("Client disconnected")
}

func (a *apmetwtracerimpl) start(_ context.Context) error {
	var err error
	etwSessionName := "Datadog APM ETW tracer"
	a.session, err = a.etw.NewSession(etwSessionName)
	if err != nil {
		return fmt.Errorf("failed to create the ETW session '%s': %v", etwSessionName, err)
	}

	a.pipeListener, err = winio.ListenPipe(namedPipePath, nil)
	if err != nil {
		return fmt.Errorf("failed to listen to named pipe '%s': %v", namedPipePath, err)
	}
	go func() {
		for {
			conn, err := a.pipeListener.Accept()
			if err != nil {
				a.log.Warnf("could not accept new client:", err)
			}
			go a.handleConnection(conn)
		}
	}()

	return a.reconfigureProvider()
}

func (a *apmetwtracerimpl) stop(_ context.Context) error {
	// No need to stop the tracing session, it's going to be automatically cleaned up
	// when the ETW component stops
	a.pipeListener.Close()
	return nil
}

func (a *apmetwtracerimpl) AddPID(pid uint64) {
	a.pids[pid] = struct{}{}
	a.reconfigureProvider()
}

func (a *apmetwtracerimpl) RemovePID(pid uint64) {
	delete(a.pids, pid)
	a.reconfigureProvider()
}

func (a *apmetwtracerimpl) reconfigureProvider() error {
	err := a.session.ConfigureProvider(a.dotNetRuntimeProviderGuid, func(cfg *etw.ProviderConfiguration) {
		cfg.TraceLevel = etw.TRACE_LEVEL_VERBOSE
		cfg.MatchAnyKeyword = 0x40004001
		pidsList := make([]uint64, 0, len(a.pids))
		for p := range a.pids {
			pidsList = append(pidsList, p)
		}
		cfg.PIDs = pidsList
	})
	if err != nil {
		return fmt.Errorf("failed to configure the Microsoft-Windows-DotNETRuntime provider: %v", err)
	}
	return nil
}
