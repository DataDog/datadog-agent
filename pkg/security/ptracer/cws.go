// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

const (
	defaultPasswdPath = "/etc/passwd"
	defaultGroupPath  = "/etc/group"
	// EnvPasswdPathOverride define the env to set to override the default passwd file path
	EnvPasswdPathOverride = "TEST_DD_PASSWD_PATH"
	// EnvGroupPathOverride define the env to set to override the default group file path
	EnvGroupPathOverride = "TEST_DD_GROUP_PATH"
)

var (
	passwdPath = defaultPasswdPath
	groupPath  = defaultGroupPath
	logger     Logger
)

// Opts defines ptracer options
type Opts struct {
	Creds            Creds
	Verbose          bool
	Debug            bool
	Async            bool
	StatsDisabled    bool
	ProcScanDisabled bool
	ScanProcEvery    time.Duration
	SeccompDisabled  bool
	AttachedCb       func()

	// internal
	mode ebpfless.Mode
}

// CWSPtracerCtx holds the ptracer internal needed variables
type CWSPtracerCtx struct {
	Tracer

	opts         *Opts
	wg           sync.WaitGroup
	cancel       context.Context
	cancelFnc    context.CancelFunc
	containerID  string
	probeAddr    string
	client       net.Conn
	clientReady  chan bool
	msgDataChan  chan []byte
	helloMsg     *ebpfless.Message
	processCache *ProcessCache
}

type syscallHandlerFunc func(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error

type shouldSendFunc func(msg *ebpfless.SyscallMsg) bool

type syscallID struct {
	ID   int
	Name string
}

type syscallHandler struct {
	ID         syscallID          // ID identifies the syscall related to this handler
	Func       syscallHandlerFunc // Func defines the entrance handler for those syscalls, can be nil
	ShouldSend shouldSendFunc     // ShouldSend checks if we should send the event regarding the syscall return value. If nil, acts as false
	RetFunc    syscallHandlerFunc // RetFunc defines the return handler for those syscalls, can be nil
}

// defaults funcs for ShouldSend:
func shouldSendAlways(_ *ebpfless.SyscallMsg) bool { return true }

func isAcceptedRetval(msg *ebpfless.SyscallMsg) bool {
	return msg.Retval >= 0 || msg.Retval == -int64(syscall.EACCES) || msg.Retval == -int64(syscall.EPERM)
}

func checkEntryPoint(path string) (string, error) {
	name, err := exec.LookPath(path)
	if err != nil {
		return "", err
	}

	name, err = filepath.Abs(name)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(name)
	if err != nil {
		return "", err
	}

	if !info.Mode().IsRegular() {
		return "", errors.New("entrypoint not a regular file")
	}

	if info.Mode()&0111 == 0 {
		return "", errors.New("entrypoint not an executable")
	}

	return name, nil
}

func initConn(probeAddr string, nbAttempts uint) (net.Conn, error) {
	tcpAddr, err := net.ResolveTCPAddr("tcp", probeAddr)
	if err != nil {
		return nil, err
	}

	var client net.Conn
	err = retry.Do(func() error {
		client, err = net.DialTCP("tcp", nil, tcpAddr)
		return err
	}, retry.Delay(time.Second), retry.Attempts(nbAttempts))
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (ctx *CWSPtracerCtx) connectClient() error {
	var err error
	ctx.client, err = initConn(ctx.probeAddr, 600)
	if err != nil {
		ctx.clientReady <- false
		logger.Errorf("connection to system-probe failed!")
		return err
	}
	ctx.clientReady <- true
	logger.Debugf("connection to system-probe initiated!")
	return nil
}

func (ctx *CWSPtracerCtx) tryReconnectClient() {
	go func() {
		_ = ctx.connectClient()
	}()
}

func (ctx *CWSPtracerCtx) initClientConnection() error {
	logger.Debugf("connection to system-probe...")
	if ctx.opts.Async {
		go func() {
			_ = ctx.connectClient()
		}()
		return nil
	}
	return ctx.connectClient()
}

func (ctx *CWSPtracerCtx) sendMsgData(data []byte) error {
	// write size
	var size [4]byte
	binary.NativeEndian.PutUint32(size[:], uint32(len(data)))
	if _, err := ctx.client.Write(size[:]); err != nil {
		return fmt.Errorf("unable to send size: %v", err)
	}

	if _, err := ctx.client.Write(data); err != nil {
		return fmt.Errorf("unable to send message: %v", err)
	}
	return nil
}

func (ctx *CWSPtracerCtx) sendMsg(msg *ebpfless.Message) error {
	logger.Debugf("sending message: %s", msg)

	if ctx.probeAddr == "" {
		return nil
	}

	data, err := msgpack.Marshal(msg)
	if err != nil {
		logger.Errorf("unable to marshal message: %v", err)
		return err
	}

	select {
	case ctx.msgDataChan <- data:
	default:
		logger.Errorf("unable to send message")
	}
	return nil
}

func (ctx *CWSPtracerCtx) waitClientToBeReady() error {
	for {
		select {
		case <-ctx.cancel.Done():
			return fmt.Errorf("Exiting")
		case ready := <-ctx.clientReady:
			if !ready {
				time.Sleep(time.Second)
				// re-init connection
				logger.Debugf("try to reconnect to system-probe...")
				ctx.tryReconnectClient()
				continue
			}

			// if ready, send an hello message
			logger.Debugf("sending message: %s", ctx.helloMsg)
			data, err := msgpack.Marshal(ctx.helloMsg)
			if err != nil {
				logger.Errorf("unable to marshal message: %v", err)
				return err
			}
			if err = ctx.sendMsgData(data); err != nil {
				logger.Debugf("error sending hallo msg: %v", err)
				ctx.tryReconnectClient()
				continue
			}
			return nil
		}
	}
}

// returns nil if client needs to be reconnected
func (ctx *CWSPtracerCtx) sendMessagesLoop() error {
	for {
		select {
		case <-ctx.cancel.Done():
			return fmt.Errorf("Exiting")
		case data := <-ctx.msgDataChan:
			if err := ctx.sendMsgData(data); err != nil {
				logger.Debugf("error sending msg: %v", err)
				ctx.msgDataChan <- data
				return nil
			}
		}
	}
}

func (ctx *CWSPtracerCtx) handleClientConnection() {
	ctx.wg.Add(1)
	go func() {
		defer ctx.wg.Done()

		for {
			// wait for the client to be ready or stopped
			if err := ctx.waitClientToBeReady(); err != nil {
				return
			}
			defer ctx.client.Close()

			// unqueue and try to send messages or wait client to be stopped
			if err := ctx.sendMessagesLoop(); err != nil {
				return
			}

			// re-init connection
			logger.Debugf("try to reconnect to system-probe...")
			ctx.tryReconnectClient()
		}
	}()

}

func registerSyscallHandlers() (map[int]syscallHandler, []string) {
	handlers := make(map[int]syscallHandler)
	syscalls := registerFIMHandlers(handlers)
	syscalls = append(syscalls, registerProcessHandlers(handlers)...)
	syscalls = append(syscalls, registerERPCHandlers(handlers)...)
	return handlers, syscalls
}

func (ctx *CWSPtracerCtx) initCtxCommon() error {
	logger = Logger{ctx.opts.Verbose, ctx.opts.Debug}

	if path := os.Getenv(EnvPasswdPathOverride); path != "" {
		passwdPath = path
	}
	if path := os.Getenv(EnvGroupPathOverride); path != "" {
		groupPath = path
	}

	ctx.clientReady = make(chan bool, 1)

	var err error
	ctx.containerID, err = getCurrentProcContainerID()
	if err != nil {
		logger.Errorf("Retrieve container ID from proc failed: %v\n", err)
	}
	containerCtx, err := newContainerContext(ctx.containerID)
	if err != nil {
		return err
	}

	ctx.syscallHandlers, ctx.PtracedSyscalls = registerSyscallHandlers()

	ctx.msgDataChan = make(chan []byte, 100000)

	ctx.processCache = NewProcessCache()

	ctx.helloMsg = &ebpfless.Message{
		Type: ebpfless.MessageTypeHello,
		Hello: &ebpfless.HelloMsg{
			Mode:             ctx.opts.mode,
			NSID:             getNSID(),
			ContainerContext: containerCtx,
			EntrypointArgs:   ctx.Args,
		},
	}

	ctx.cancel, ctx.cancelFnc = context.WithCancel(context.Background())
	return nil
}

func initCWSPtracerWrapp(args []string, envs []string, probeAddr string, opts Opts) (*CWSPtracerCtx, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("an executable is required")
	}
	entry, err := checkEntryPoint(args[0])
	if err != nil {
		return nil, err
	}

	ctx := &CWSPtracerCtx{
		opts:      &opts,
		probeAddr: probeAddr,
		Tracer: Tracer{
			entry: entry,
			Args:  args,
			Envs:  envs,
		},
	}
	ctx.opts.mode = ebpfless.WrappedMode
	if err := ctx.initCtxCommon(); err != nil {
		return nil, err
	}

	logger.Debugf("Run %s %v [%s]", ctx.entry, args, os.Getenv("DD_CONTAINER_ID"))

	return ctx, nil
}

func initCWSPtracerAttach(pids []int, probeAddr string, opts Opts) (*CWSPtracerCtx, error) {
	ctx := &CWSPtracerCtx{
		opts:      &opts,
		probeAddr: probeAddr,
		Tracer: Tracer{
			PIDs: pids,
		},
	}
	ctx.opts.mode = ebpfless.AttachedMode
	// force to true, can't use seccomp with the attach mode
	ctx.opts.SeccompDisabled = true
	if err := ctx.initCtxCommon(); err != nil {
		return nil, err
	}

	// collect tracees via proc
	for _, pid := range pids {
		proc, err := collectProcess(int32(pid))
		if err != nil {
			return nil, err
		}
		if msg, err := procToMsg(proc); err == nil {
			if err := ctx.sendMsg(msg); err != nil {
				return nil, err
			}
		}
	}

	mode := "seccomp"
	if opts.SeccompDisabled {
		mode = "standard"
	}
	logger.Logf("Run %s %v [%s] using `%s` mode", ctx.entry, ctx.Args, os.Getenv("DD_CONTAINER_ID"), mode)

	return ctx, nil
}

// CWSCleanup cleans up the ptracer
func (ctx *CWSPtracerCtx) CWSCleanup() {
	ctx.cancelFnc()
	ctx.wg.Wait()
	close(ctx.msgDataChan)
	close(ctx.clientReady)
}

// Attach attach the ptracer
func Attach(pids []int, probeAddr string, opts Opts) error {
	ctx, err := initCWSPtracerAttach(pids, probeAddr, opts)
	if err != nil {
		return err
	}

	if err := ctx.AttachTracer(); err != nil {
		return err
	}

	if opts.AttachedCb != nil {
		opts.AttachedCb()
	}

	return ctx.StartCWSPtracer()
}

// Wrap the executable
func Wrap(args []string, envs []string, probeAddr string, opts Opts) error {
	ctx, err := initCWSPtracerWrapp(args, envs, probeAddr, opts)
	if err != nil {
		return err
	}

	if err := ctx.NewTracer(); err != nil {
		return err
	}

	return ctx.StartCWSPtracer()
}

// StartCWSPtracer start the ptracer
func (ctx *CWSPtracerCtx) StartCWSPtracer() error {
	defer ctx.CWSCleanup()

	if ctx.probeAddr != "" {
		if err := ctx.initClientConnection(); err != nil {
			return err
		}
	}

	if ctx.probeAddr == "" {
		logger.Debugf("sending message: %s", ctx.helloMsg)
	} else {
		ctx.handleClientConnection()
	}

	if !ctx.opts.ProcScanDisabled {
		ctx.startScanProcfs()
	}

	if err := ctx.Trace(); err != nil {
		return err
	}

	// let a few queued message being send
	time.Sleep(time.Second)

	return nil
}
