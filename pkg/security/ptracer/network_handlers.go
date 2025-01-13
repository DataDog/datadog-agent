// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
	"bytes"
	"encoding/binary"
	"errors"
	"golang.org/x/sys/unix"
	"net"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/proto/ebpfless"
)

func registerNetworkHandlers(handlers map[int]syscallHandler) []string {
	processHandlers := []syscallHandler{
		{
			ID:         syscallID{ID: AcceptNr, Name: "accept"},
			Func:       nil,
			ShouldSend: shouldSendAccept,
			RetFunc:    handleAcceptRet,
		},
		{
			ID:         syscallID{ID: Accept4Nr, Name: "accept4"},
			Func:       nil,
			ShouldSend: shouldSendAccept,
			RetFunc:    handleAcceptRet,
		},
		{
			ID:         syscallID{ID: BindNr, Name: "bind"},
			Func:       handleBind,
			ShouldSend: shouldSendBind,
			RetFunc:    handleBindRet,
		},
		{
			ID:         syscallID{ID: ConnectNr, Name: "connect"},
			Func:       handleConnect,
			ShouldSend: shouldSendConnect,
			RetFunc:    handleConnectRet,
		},
		{
			ID:         syscallID{ID: SocketNr, Name: "socket"},
			Func:       handleSocket,
			ShouldSend: nil,
			RetFunc:    handleSocketRet,
		},
	}

	syscallList := []string{}
	for _, h := range processHandlers {
		if h.ID.ID >= 0 { // insert only available syscalls
			handlers[h.ID.ID] = h
			syscallList = append(syscallList, h.ID.Name)
		}
	}
	return syscallList
}

type AddrInfo struct {
	ip   net.IP
	port uint16
	af   uint16
}

func parseAddrInfo(tracer *Tracer, process *Process, argPos int, regs syscall.PtraceRegs, addrlen int32) (*AddrInfo, error) {
	if addrlen < 16 {
		return nil, errors.New("invalid address length")
	}

	data, err := tracer.ReadArgData(process.Pid, regs, 1, uint(addrlen))

	var addr AddrInfo

	if err != nil {
		return nil, err
	}

	buf := bytes.NewReader(data[0:2])
	binary.Read(buf, binary.LittleEndian, &addr.af)

	buf = bytes.NewReader(data[2:4])
	binary.Read(buf, binary.BigEndian, &addr.port)

	if addr.af == unix.AF_INET {
		data, err := tracer.ReadArgData(process.Pid, regs, 1, 16)
		if err != nil {
			return nil, err
		}
		addr.ip = data[4:8]
	} else if addr.af == unix.AF_INET6 {
		if addrlen < 28 {
			return nil, errors.New("invalid address length")
		}

		data, err := tracer.ReadArgData(process.Pid, regs, 1, 28)
		if err != nil {
			return nil, err
		}
		addr.ip = data[8:24]
	} else {
		return nil, errors.New("unsupported address family")
	}

	return &addr, nil
}

func handleBind(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	msg.Type = ebpfless.SyscallTypeBind
	socketfd := tracer.ReadArgUint32(regs, 0)

	socketInfo, ok := process.FdToSocket[int32(socketfd)]
	if !ok {
		return errors.New("unable to find socket")
	}

	if socketInfo.AddressFamily == unix.AF_UNIX {
		msg.Bind = &ebpfless.BindSyscallMsg{
			AddressFamily: unix.AF_UNIX,
			Addr:          net.IP{},
			Port:          0,
			Protocol:      0,
		}
		return nil
	}

	var addrlen int32
	if socketInfo.AddressFamily == unix.AF_INET {
		addrlen = 16
	} else if socketInfo.AddressFamily == unix.AF_INET6 {
		addrlen = 28
	}

	addr, err := parseAddrInfo(tracer, process, 1, regs, addrlen)
	if err != nil {
		return err
	}

	msg.Bind = &ebpfless.BindSyscallMsg{
		AddressFamily: addr.af,
		Addr:          addr.ip,
		Port:          addr.port,
		Protocol:      socketInfo.Protocol,
	}

	return nil
}

func handleConnect(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	addrlen := tracer.ReadArgInt32(regs, 2)
	addr, err := parseAddrInfo(tracer, process, 1, regs, int32(addrlen))

	if err != nil {
		return err
	}
	socketfd := int32(tracer.ReadArgUint32(regs, 0))

	msg.Type = ebpfless.SyscallTypeConnect
	msg.Retval = tracer.ReadRet(regs)

	msg.Connect = &ebpfless.ConnectSyscallMsg{
		AddressFamily: addr.af,
		Addr:          addr.ip,
		Port:          addr.port,
		Protocol:      process.FdToSocket[socketfd].Protocol,
	}

	return nil
}

func handleSocket(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	lastSocket := &SocketInfo{
		AddressFamily: uint16(tracer.ReadArgInt32(regs, 0)),
	}

	if lastSocket.AddressFamily != unix.AF_INET && lastSocket.AddressFamily != unix.AF_INET6 && lastSocket.AddressFamily != unix.AF_UNIX {
		return nil
	}

	protocol := int16(tracer.ReadArgInt32(regs, 1))
	// This argument can be masked, so just get what we need
	protocol &= 0b1111

	switch protocol {
	case unix.SOCK_STREAM:
		lastSocket.Protocol = unix.IPPROTO_TCP
	case unix.SOCK_DGRAM:
		lastSocket.Protocol = unix.IPPROTO_UDP
	default:
		return nil
	}

	process.LastSocket = lastSocket
	return nil
}

// Handle returns
func handleAcceptRet(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	addrlen, _ := tracer.ReadArgInt32Ptr(process.Pid, regs, 2)

	addr, err := parseAddrInfo(tracer, process, 1, regs, addrlen)

	if err != nil {
		return err
	}

	msg.Type = ebpfless.SyscallTypeAccept
	msg.Retval = tracer.ReadRet(regs)

	msg.Accept = &ebpfless.AcceptSyscallMsg{
		AddressFamily: addr.af,
		Addr:          addr.ip,
		Port:          addr.port,
	}

	return nil
}

func handleBindRet(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	msg.Retval = tracer.ReadRet(regs)
	return nil
}

func handleConnectRet(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	msg.Retval = tracer.ReadRet(regs)
	return nil
}

func handleSocketRet(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, disableStats bool) error {
	ret := int32(tracer.ReadRet(regs))

	if process.LastSocket != nil && ret != -1 {
		process.FdToSocket[ret] = *process.LastSocket
	}

	process.LastSocket = nil
	return nil
}

// Should send messages
func shouldSendConnect(msg *ebpfless.SyscallMsg) bool {
	if msg.Connect != nil {
		return true
	}
	return false
}

func shouldSendAccept(msg *ebpfless.SyscallMsg) bool {
	if msg.Accept != nil {
		return true
	}
	return false
}

func shouldSendBind(msg *ebpfless.SyscallMsg) bool {
	if msg.Bind != nil {
		return true
	}
	return false
}
