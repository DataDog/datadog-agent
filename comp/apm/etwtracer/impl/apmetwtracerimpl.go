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
	"errors"
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
	"time"
	"unsafe"
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

// pidContext wraps a named-pipe connection and a last-seen
// timestamp for a given PID.
type pidContext struct {
	conn     net.Conn
	lastSeen time.Time
}

type pidMap = map[uint64]pidContext

func newApmEtwTracerImpl(deps dependencies) (apmetwtracer.Component, error) {
	// Microsoft-Windows-DotNETRuntime
	guid, _ := windows.GUIDFromString("{E13C0D23-CCBC-4E12-931B-D9CC2EEE27E4}")

	apmEtwTracer := &apmetwtracerimpl{
		pids:                      make(pidMap),
		dotNetRuntimeProviderGuid: guid,
		log:                       deps.Log,
		etw:                       deps.Etw,
		stopGarbageCollector:      make(chan struct{}),
	}

	// Cache the magic header
	for idx := range magicHeaderString {
		apmEtwTracer.magic[idx] = magicHeaderString[idx]
	}

	deps.Lc.Append(fx.Hook{OnStart: apmEtwTracer.start, OnStop: apmEtwTracer.stop})
	return apmEtwTracer, nil
}

type apmetwtracerimpl struct {
	session                   etw.Session
	dotNetRuntimeProviderGuid windows.GUID

	pids pidMap

	pipeListener         net.Listener
	log                  log.Component
	etw                  etw.Component
	magic                [14]byte
	stopGarbageCollector chan struct{}
}

type header struct {
	Magic           [14]byte
	Size            uint16
	CommandResponse uint8
}

type clrEvent struct {
	header
	event etw.DDEtwEvent
}

const (
	magicHeaderString   = "DD_ETW_IPC_V1"
	serverNamedPipePath = "\\\\.\\pipe\\DD_ETW_DISPATCHER"
	clientNamedPipePath = "\\\\.\\pipe\\DD_ETW_CLIENT_%d"
	headerSize          = 17
	OkResponse          = 0
	RegisterCommand     = 1
	UnregisterCommand   = 2
	ClrEventResponse    = 16
	ErrorResponse       = 255
)

type win32MessageBytePipe interface {
	CloseWrite() error
}

func (a *apmetwtracerimpl) handleConnection(c net.Conn) {
	// calls https://github.com/microsoft/go-winio/blob/e6aebd619a7278545b11188a0e21babea6b94182/pipe.go#L147
	// which closes a pipe in message-mode
	defer c.(win32MessageBytePipe).CloseWrite()

	a.log.Debugf("client connected [%s]", c.RemoteAddr().Network())
	for {
		h := header{}
		err := binary.Read(c, binary.LittleEndian, &h)

		// Client disconnected
		if err == io.EOF {
			return
		}

		if err != nil {
			a.log.Errorf("Read error: %v", err)
			return
		}

		magicStr := string(h.Magic[:13]) // Don't count last byte
		if magicStr != magicHeaderString {
			a.log.Errorf("Invalid header: %s", magicStr)
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
			a.log.Errorf("Read error: %v", err)
			return
		}

		switch h.CommandResponse {
		case RegisterCommand:
			a.log.Infof("Registering process with ID %d", pid)
			err = a.AddPID(pid)
			if err != nil {
				a.log.Errorf("Failed to reconfigure the ETW provider for PID %d: %v", pid, err)
				h.CommandResponse = ErrorResponse
			} else {
				h.CommandResponse = OkResponse
			}
			break
		case UnregisterCommand:
			a.log.Infof("Unregistering process with ID %d", pid)
			err = a.RemovePID(pid)
			if err != nil {
				a.log.Errorf("Failed to reconfigure the ETW provider for PID %d: %v", pid, err)
				h.CommandResponse = ErrorResponse
			} else {
				h.CommandResponse = OkResponse
			}
			break
		default:
			a.log.Infof("Unsupported command %d", h.CommandResponse)
		}
		h.Size = headerSize
		
		err = binary.Write(c, binary.LittleEndian, &h)

		// Client disconnected
		if err == io.EOF {
			return
		}

		if err != nil {
			a.log.Errorf("Read error: %v", err)
			return
		}
	}
}

func (a *apmetwtracerimpl) start(_ context.Context) error {
	a.log.Infof("Starting Datadog APM ETW tracer component")
	var err error
	etwSessionName := "Datadog APM ETW tracer"
	a.session, err = a.etw.NewSession(etwSessionName)
	if err != nil {
		return fmt.Errorf("failed to create the ETW session '%s': %v", etwSessionName, err)
	}

	a.pipeListener, err = winio.ListenPipe(serverNamedPipePath, &winio.PipeConfig{
		MessageMode: true,
	})
	if err != nil {
		return fmt.Errorf("failed to listen to named pipe '%s': %v", serverNamedPipePath, err)
	}
	go func() {
		for {
			conn, err := a.pipeListener.Accept()
			if err != nil {
				// net.ErrClosed is returned when pipeListener is Close()'d
				if err != net.ErrClosed {
					a.log.Warnf("could not accept new client:", err)
				}
				return
			}
			go a.handleConnection(conn)
		}
	}()

	startTracingErrorChan := make(chan error)
	go func() {
		// StartTracing blocks the caller
		startTracingErr := a.session.StartTracing(func(e *etw.DDEtwEvent) {
			a.log.Infof("received event %d for PID %d", e.Id, e.Pid)
			pid := uint64(e.Pid)
			var pidCtx pidContext
			var ok bool
			if pidCtx, ok = a.pids[pid]; !ok {
				// We may still be receiving events a few moments
				// after a process un-registers itself, no need to log anything here.
				return
			}
			pidCtx.lastSeen = time.Now()
			ev := clrEvent{
				header: header{
					Magic:           a.magic,
					CommandResponse: ClrEventResponse,
				},
				event: *e,
			}
			ev.header.Size = uint16(unsafe.Sizeof(ev))
			writeErr := binary.Write(pidCtx.conn, binary.LittleEndian, ev)
			if writeErr != nil {
				a.log.Warnf("could not write ETW event for PID %d, %v", pid, writeErr)
			}
		})
		// This error will be returned to the caller if it happens withing 100ms
		// otherwise we assume StartTracing worked and whatever error message is
		// returned here will be lost.
		// That's ok because StartTracing should only return when StopTracing is called
		// at the end of the program execution.
		startTracingErrorChan <- startTracingErr
	}()

	// Start a garbage collection goroutine to cleanup periodically processes
	// that might have crashed without unregistering themselves.
	go func() {
		for {
			select {
			case <-a.stopGarbageCollector:
				return
			case <-time.After(10 * time.Second):
				// Every 10 seconds, check if any PID has become a zombie
				now := time.Now()
				for pid, pidCtx := range a.pids {
					if now.Sub(pidCtx.lastSeen) > time.Minute {
						// No events received for more than 1 minute,
						// remove this PID
						a.log.Infof("removing PID %d for being idle for > 1 minute", pid)
						a.RemovePID(pid)
					}
				}
			}
		}
	}()

	// Since we can only know if StartTracing failed after calling it,
	// and it is a blocking call, we wait for 100ms to see if it failed.
	// Otherwise, we don't want to block the start method for longer than that.
	select {
	case err = <-startTracingErrorChan:
		return err
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

func (a *apmetwtracerimpl) stop(_ context.Context) error {
	a.log.Infof("Stopping Datadog APM ETW tracer component")
	a.stopGarbageCollector <- struct{}{}
	err := a.session.StopTracing()
	err = errors.Join(err, a.pipeListener.Close())
	return err
}

func (a *apmetwtracerimpl) AddPID(pid uint64) error {
	c, err := winio.DialPipe(fmt.Sprintf(clientNamedPipePath, pid), nil)
	if err != nil {
		return err
	}
	a.pids[pid] = pidContext{
		conn:     c,
		lastSeen: time.Now(),
	}
	a.reconfigureProvider()
	if len(a.pids) > 0 {
		return a.session.EnableProvider(a.dotNetRuntimeProviderGuid)
	}
	return nil
}

func (a *apmetwtracerimpl) RemovePID(pid uint64) error {
	var pidCtx pidContext
	var ok bool
	if pidCtx, ok = a.pids[pid]; !ok {
		return fmt.Errorf("could not find PID %d in PID list", pid)
	}
	pidCtx.conn.Close()
	delete(a.pids, pid)
	a.reconfigureProvider()
	if len(a.pids) <= 0 {
		return a.session.DisableProvider(a.dotNetRuntimeProviderGuid)
	}
	return nil
}

func (a *apmetwtracerimpl) reconfigureProvider() {
	a.session.ConfigureProvider(a.dotNetRuntimeProviderGuid, func(cfg *etw.ProviderConfiguration) {
		cfg.TraceLevel = etw.TRACE_LEVEL_VERBOSE
		cfg.MatchAnyKeyword = 0x40004001
		pidsList := make([]uint64, 0, len(a.pids))
		for p := range a.pids {
			pidsList = append(pidsList, p)
		}
		cfg.PIDs = pidsList
	})
}
