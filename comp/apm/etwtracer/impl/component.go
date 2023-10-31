// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package apmetwtracerimpl provides a component for the .Net tracer application
package apmetwtracerimpl

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/apm/etwtracer"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/etw"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/Microsoft/go-winio"
	"go.uber.org/fx"
	"golang.org/x/sys/windows"
	"io"
	"net"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newApmEtwTracerImpl),
)

type dependencies struct {
	fx.In
	Lc  fx.Lifecycle
	Log log.Component
	Etw etw.Component
}

func newApmEtwTracerImpl(deps dependencies) (apmetwtracer.Component, error) {
	// Microsoft-Windows-DotNETRuntime
	guid, _ := windows.GUIDFromString("{E13C0D23-CCBC-4E12-931B-D9CC2EEE27E4}")

	apmEtwTracer := &apmetwtracerimpl{
		log:                       deps.Log,
		etw:                       deps.Etw,
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
	Magic           [14]byte
	Size            uint16
	CommandResponse uint8
}

const (
	namedPipePath = "\\\\.\\pipe\\DD_ETW_DISPATCHER"
	bufSize       = 25
	Register      = 1
	Unregister    = 2
	ClrEvent      = 16
)

type win32MessageBytePipe interface {
	CloseWrite() error
}

func (a *apmetwtracerimpl) handleConnection(c net.Conn) {
	// calls https://github.com/microsoft/go-winio/blob/e6aebd619a7278545b11188a0e21babea6b94182/pipe.go#L147
	// which closes a pipe in message-mode
	defer c.(win32MessageBytePipe).CloseWrite()

	a.log.Debugf("client connected [%s]\n", c.RemoteAddr().Network())
	for {
		h := header{}
		err := binary.Read(c, binary.LittleEndian, &h)

		// Client disconnected
		if err == io.EOF {
			return
		}

		if err != nil {
			a.log.Errorf("Read error: %v\n", err)
			return
		}

		magicStr := string(h.Magic[:13]) // Don't count last byte
		if magicStr != "DD_ETW_IPC_V1" {
			a.log.Errorf("Invalid header: %s\n", magicStr)
			return
		}

		// Read pid
		var pid uint64
		err = binary.Read(c, binary.LittleEndian, &pid)

		// Client disconnected
		if err == io.EOF {
			return
		}

		if err != nil {
			a.log.Errorf("Read error: %v\n", err)
			return
		}

		switch h.CommandResponse {
		case Register:
			a.log.Debugf("Registering process with ID %d\n", pid)
			err = a.AddPID(pid)
			if err != nil {
				a.log.Errorf("Failed to reconfigure the ETW provider for PID %d: %v\n", pid, err)
				return
			}
			break
		case Unregister:
			a.log.Debugf("Unregistering process with ID %d\n", pid)
			err = a.RemovePID(pid)
			if err != nil {
				a.log.Errorf("Failed to reconfigure the ETW provider for PID %d: %v\n", pid, err)
				return
			}
			break
		}

		h.CommandResponse = 0 // ok
		h.Size = 17           // header = 17

		err = binary.Write(c, binary.LittleEndian, &h)

		// Client disconnected
		if err == io.EOF {
			return
		}

		if err != nil {
			a.log.Errorf("Read error: %v\n", err)
			return
		}
	}
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

	err = a.reconfigureProvider()
	if err != nil {
		return err
	}

	return a.session.StartTracing(func(e *etw.DDEtwEvent) {
		a.log.Infof("received event %d from %v\n", e.Id, e.ProviderId)
	})
}

func (a *apmetwtracerimpl) stop(_ context.Context) error {
	// No need to stop the tracing session, it's going to be automatically cleaned up
	// when the ETW component stops
	a.pipeListener.Close()
	return nil
}

func (a *apmetwtracerimpl) AddPID(pid uint64) error {
	a.pids[pid] = struct{}{}
	return a.reconfigureProvider()
}

func (a *apmetwtracerimpl) RemovePID(pid uint64) error {
	delete(a.pids, pid)
	return a.reconfigureProvider()
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
