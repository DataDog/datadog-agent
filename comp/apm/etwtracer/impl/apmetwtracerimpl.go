// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package apmetwtracerimpl provides a component for the .Net tracer application
package apmetwtracerimpl

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/apm/etwtracer"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/etw"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	etwutil "github.com/DataDog/datadog-agent/pkg/util/winutil/etw"
	"github.com/Microsoft/go-winio"
	"github.com/alecthomas/units"
	"go.uber.org/fx"
	"golang.org/x/sys/windows"
	"io"
	"net"
	"os"
	"sync"
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

// pidContext holds the necessary context for each PID that is monitored by this integration.
type pidContext struct {
	conn net.Conn
}

type pidMap = map[uint64]pidContext

func newApmEtwTracerImpl(deps dependencies) (apmetwtracer.Component, error) {
	// Microsoft-Windows-DotNETRuntime
	guid, _ := windows.GUIDFromString("{E13C0D23-CCBC-4E12-931B-D9CC2EEE27E4}")

	apmEtwTracer := &apmetwtracerimpl{
		pids:                      make(pidMap),
		dotNetRuntimeProviderGUID: guid,
		log:                       deps.Log,
		etw:                       deps.Etw,
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
	dotNetRuntimeProviderGUID windows.GUID

	pids     pidMap
	pidMutex sync.Mutex

	pipeListener net.Listener
	log          log.Component
	etw          etw.Component
	magic        [14]byte
}

type header struct {
	Magic           [14]byte
	Size            uint16
	CommandResponse uint8
}

const (
	magicHeaderString   = "DD_ETW_IPC_V1"
	serverNamedPipePath = "\\\\.\\pipe\\DD_ETW_DISPATCHER"
	clientNamedPipePath = "\\\\.\\pipe\\DD_ETW_CLIENT_%d"
	headerSize          = 17 // byte
	okResponse          = 0
	registerCommand     = 1
	unregisterCommand   = 2
	clrEventResponse    = 16
	errorResponse       = 255
	payloadBufferSize   = 64 * units.Kilobyte
)

type win32MessageBytePipe interface {
	CloseWrite() error
}

func (a *apmetwtracerimpl) readBinary(c net.Conn, data any) error {
	// Don't set a deadline for reading, some clients might be silent for a long time
	// and keep the connection open.
	err := binary.Read(c, binary.LittleEndian, data)
	if errors.Is(err, io.EOF) {
		a.log.Debugf("Client disconnected [%s]", c.RemoteAddr().Network())
		return err
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		a.log.Debugf("Client timed-out [%s]", c.RemoteAddr().Network())
		return err
	}
	if err != nil {
		a.log.Errorf("Read error: %v", err)
		return err
	}
	return nil
}

func (a *apmetwtracerimpl) writeBinary(c net.Conn, data any) error {
	err := c.SetWriteDeadline(time.Now().Add(30 * time.Second))
	if err != nil {
		return err
	}
	err = binary.Write(c, binary.LittleEndian, data)
	if errors.Is(err, io.EOF) {
		a.log.Debugf("Client disconnected [%s]", c.RemoteAddr().Network())
		return err
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		a.log.Debugf("Client timed-out [%s]", c.RemoteAddr().Network())
		return err
	}
	if err != nil {
		a.log.Errorf("Write error: %v", err)
		return err
	}
	return nil
}

func (a *apmetwtracerimpl) handleConnection(c net.Conn) {
	// calls https://github.com/microsoft/go-winio/blob/e6aebd619a7278545b11188a0e21babea6b94182/pipe.go#L147
	// which closes a pipe in message-mode
	defer func(pipe win32MessageBytePipe) {
		_ = pipe.CloseWrite()
	}(c.(win32MessageBytePipe))

	a.log.Debugf("Client connected [%s]", c.RemoteAddr().Network())
	for {
		h := header{}
		err := a.readBinary(c, &h)
		if err != nil {
			// Error is handled in readBinary
			return
		}

		if !bytes.Equal(a.magic[:], h.Magic[:]) {
			a.log.Errorf("Invalid header: %s", string(h.Magic[:]))
			return
		}

		// Read pid
		var pid uint64
		err = a.readBinary(c, &pid)
		if err != nil {
			// Error is handled in readBinary
			return
		}

		switch h.CommandResponse {
		case registerCommand:
			a.log.Infof("Registering process with ID %d", pid)
			a.pidMutex.Lock()
			err = a.addPID(pid)
			a.pidMutex.Unlock()
			if err != nil {
				a.log.Errorf("Failed to reconfigure the ETW provider for PID %d: %v", pid, err)
				h.CommandResponse = errorResponse
			} else {
				h.CommandResponse = okResponse
			}
		case unregisterCommand:
			a.log.Infof("Unregistering process with ID %d", pid)
			a.pidMutex.Lock()
			err = a.removePID(pid)
			a.pidMutex.Unlock()
			if err != nil {
				a.log.Errorf("Failed to reconfigure the ETW provider for PID %d: %v", pid, err)
				h.CommandResponse = errorResponse
			} else {
				h.CommandResponse = okResponse
			}
		default:
			a.log.Infof("Unsupported command %d", h.CommandResponse)
		}
		h.Size = headerSize

		// Error is handled in writeBinary
		_ = a.writeBinary(c, &h)
	}
}

func (a *apmetwtracerimpl) start(_ context.Context) error {
	a.log.Infof("Starting Datadog APM ETW tracer component")
	var err error
	etwSessionName := "Datadog APM ETW tracer"
	a.session, err = a.etw.NewSession(etwSessionName)
	if err != nil {
		a.log.Errorf("Failed to create the ETW session '%s': %v", etwSessionName, err)
		// Don't fail the Agent startup
		return nil
	}

	a.pipeListener, err = winio.ListenPipe(serverNamedPipePath, &winio.PipeConfig{
		// https://learn.microsoft.com/en-us/windows/win32/secauthz/security-descriptor-string-format
		// https://learn.microsoft.com/en-us/windows/win32/secauthz/ace-strings
		// https://learn.microsoft.com/en-us/windows/win32/secauthz/sid-strings
		//
		// D:dacl_flags(ace_type;ace_flags;rights;object_guid;inherit_object_guid;account_sid;(resource_attribute))
		// 	dacl_flags:
		//		"P": SDDL_PROTECTED
		//	ace_type:
		//		"A": SDDL_ACCESS_ALLOWED
		// rights:
		//		"GA": SDDL_GENERIC_ALL
		// account_sid:
		//		"WD": Everyone
		SecurityDescriptor: "D:P(A;;GA;;;WD)",
		MessageMode:        true,
	})
	if err != nil {
		a.log.Errorf("Failed to listen to named pipe '%s': %v", serverNamedPipePath, err)
		// Don't fail the Agent startup
		return nil
	}
	go func() {
		for {
			conn, err := a.pipeListener.Accept()
			if err != nil {
				// net.ErrClosed is returned when pipeListener is Close()'d
				if err != net.ErrClosed {
					a.log.Warnf("Could not accept new client:", err)
				}
				return
			}
			go a.handleConnection(conn)
		}
	}()

	go a.doTrace()

	return nil
}

func (a *apmetwtracerimpl) doTrace() {
	var payloadBuffer bytes.Buffer
	// preallocate a fixed size
	payloadBuffer.Grow(int(payloadBufferSize))

	// StartTracing blocks the caller
	_ = a.session.StartTracing(func(e *etw.DDEventRecord) {
		a.log.Debugf("Received event %d for PID %d", e.EventHeader.EventDescriptor.ID, e.EventHeader.ProcessID)
		pid := uint64(e.EventHeader.ProcessID)

		a.pidMutex.Lock()
		var err error
		defer func() {
			if err != nil {
				a.log.Errorf("Could not write ETW event for PID %d, %v", pid, err)
				err = a.removePID(pid)
				if err != nil {
					a.log.Errorf("Could not remove PID %d, %v", pid, err)
				}
			}
			defer a.pidMutex.Unlock()
		}()

		var pidCtx pidContext
		var ok bool
		if pidCtx, ok = a.pids[pid]; !ok {
			// We may still be receiving events a few moments
			// after a process un-registers itself, no need to log anything here.
			return
		}

		payloadBuffer.Reset()
		binWriter := bufio.NewWriter(&payloadBuffer)
		err = binary.Write(binWriter, binary.LittleEndian, header{
			Magic:           a.magic,
			CommandResponse: clrEventResponse,
			Size:            uint16(headerSize+unsafe.Sizeof(e.EventHeader)+unsafe.Sizeof(e.UserDataLength)) + e.UserDataLength,
		})
		if err != nil {
			return
		}
		err = binary.Write(binWriter, binary.LittleEndian, e.EventHeader)
		if err != nil {
			return
		}
		err = binary.Write(binWriter, binary.LittleEndian, e.UserDataLength)
		if err != nil {
			return
		}
		_, err = binWriter.Write(etwutil.GoBytes(unsafe.Pointer(e.UserData), int(e.UserDataLength)))
		if err != nil {
			return
		}
		err = binWriter.Flush()
		if err != nil {
			return
		}
		_, err = pidCtx.conn.Write(payloadBuffer.Bytes())
		if err != nil {
			return
		}
	})
}

func (a *apmetwtracerimpl) stop(_ context.Context) error {
	a.log.Infof("Stopping Datadog APM ETW tracer component")
	err := a.session.StopTracing()
	err = errors.Join(err, a.pipeListener.Close())
	return err
}

func (a *apmetwtracerimpl) addPID(pid uint64) error {
	c, err := winio.DialPipe(fmt.Sprintf(clientNamedPipePath, pid), nil)
	if err != nil {
		return err
	}
	a.pids[pid] = pidContext{
		conn: c,
	}
	return a.reconfigureProvider()
}

func (a *apmetwtracerimpl) removePID(pid uint64) error {
	var pidCtx pidContext
	var ok bool
	if pidCtx, ok = a.pids[pid]; !ok {
		return fmt.Errorf("could not find PID %d in PID list", pid)
	}
	delete(a.pids, pid)
	pidCtx.conn.Close()

	return a.reconfigureProvider()
}

func (a *apmetwtracerimpl) reconfigureProvider() error {
	pidsList := make([]uint64, 0, len(a.pids))
	for p := range a.pids {
		pidsList = append(pidsList, p)
	}

	a.session.ConfigureProvider(a.dotNetRuntimeProviderGUID, func(cfg *etw.ProviderConfiguration) {
		cfg.TraceLevel = etw.TRACE_LEVEL_VERBOSE
		cfg.MatchAnyKeyword = 0x40004001
		cfg.PIDs = pidsList
	})

	if len(pidsList) > 0 {
		return a.session.EnableProvider(a.dotNetRuntimeProviderGUID)
	}
	return a.session.DisableProvider(a.dotNetRuntimeProviderGUID)
}
