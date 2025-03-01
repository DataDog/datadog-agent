// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package ptracer holds the start command of CWS injector
package ptracer

import (
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
			Func:       handleAccept,
			ShouldSend: shouldSendAccept,
			RetFunc:    handleAcceptRet,
		},
		{
			ID:         syscallID{ID: Accept4Nr, Name: "accept4"},
			Func:       handleAccept,
			ShouldSend: shouldSendAccept,
			RetFunc:    handleAcceptRet,
		},
		{
			ID:         syscallID{ID: BindNr, Name: "bind"},
			Func:       handleBind,
			ShouldSend: shouldSendBind,
			RetFunc:    nil,
		},
		{
			ID:         syscallID{ID: ConnectNr, Name: "connect"},
			Func:       handleConnect,
			ShouldSend: shouldSendConnect,
			RetFunc:    nil,
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

type addrInfo struct {
	ip   net.IP
	port uint16
	af   uint16
}

func parseAddrInfo(tracer *Tracer, process *Process, regs syscall.PtraceRegs, addrlen int32) (*addrInfo, error) {
	if addrlen < 16 {
		return nil, errors.New("invalid address length")
	}

	if addrlen > 28 {
		addrlen = 28
	}

	data, err := tracer.ReadArgData(process.Pid, regs, 1, uint(addrlen))
	if err != nil {
		return nil, err
	}

	var addr addrInfo

	addr.af = binary.NativeEndian.Uint16(data[0:2])
	addr.port = binary.BigEndian.Uint16(data[2:4])

	if addr.af == unix.AF_INET {
		addr.ip = data[4:8]
	} else if addr.af == unix.AF_INET6 {
		if addrlen < 28 {
			return nil, errors.New("invalid address length")
		}

		addr.ip = data[8:24]
	} else {
		return nil, errors.New("unsupported address family")
	}

	return &addr, nil
}

func handleBind(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	socketfd := tracer.ReadArgUint32(regs, 0)

	socketInfo, ok := process.FdToSocket[int32(socketfd)]
	if !ok {
		return errors.New("unable to find socket")
	}

	var addrlen int32
	if socketInfo.AddressFamily == unix.AF_INET {
		addrlen = 16
	} else if socketInfo.AddressFamily == unix.AF_INET6 {
		addrlen = 28
	} else if socketInfo.AddressFamily == unix.AF_UNIX {
		addrlen = 0
	} else {
		return errors.New("unsupported address family")
	}

	if socketInfo.AddressFamily == unix.AF_UNIX {
		msg.Type = ebpfless.SyscallTypeBind
		msg.Bind = &ebpfless.BindSyscallMsg{
			MsgSocketInfo: ebpfless.MsgSocketInfo{
				AddressFamily: unix.AF_UNIX,
				Addr:          net.IP{},
				Port:          0,
			},
			Protocol: 0,
		}
		return nil
	}

	addr, err := parseAddrInfo(tracer, process, regs, addrlen)
	if err != nil {
		return err
	}

	msg.Type = ebpfless.SyscallTypeBind
	msg.Bind = &ebpfless.BindSyscallMsg{
		MsgSocketInfo: ebpfless.MsgSocketInfo{
			AddressFamily: addr.af,
			Addr:          addr.ip,
			Port:          addr.port,
		},
		Protocol: socketInfo.Protocol,
	}

	socketInfo.BoundToPort = addr.port
	process.FdToSocket[int32(socketfd)] = socketInfo

	return nil
}

func handleConnect(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	addrlen := tracer.ReadArgInt32(regs, 2)
	addr, err := parseAddrInfo(tracer, process, regs, int32(addrlen))

	if err != nil {
		return err
	}
	socketfd := int32(tracer.ReadArgUint32(regs, 0))

	m, ok := process.FdToSocket[socketfd]

	if !ok {
		return errors.New("unable to find protocol")
	}

	msg.Type = ebpfless.SyscallTypeConnect
	msg.Connect = &ebpfless.ConnectSyscallMsg{
		MsgSocketInfo: ebpfless.MsgSocketInfo{
			AddressFamily: addr.af,
			Addr:          addr.ip,
			Port:          addr.port,
		},
		Protocol: m.Protocol,
	}

	return nil
}

func handleSocket(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	socketMsg := &ebpfless.SocketSyscallFakeMsg{
		AddressFamily: uint16(tracer.ReadArgInt32(regs, 0)),
	}

	if socketMsg.AddressFamily != unix.AF_INET && socketMsg.AddressFamily != unix.AF_INET6 && socketMsg.AddressFamily != unix.AF_UNIX {
		return nil
	}

	protocol := int16(tracer.ReadArgInt32(regs, 1))
	// This argument can be masked, so just get what we need
	protocol &= 0b1111

	switch protocol {
	case unix.SOCK_STREAM:
		socketMsg.Protocol = unix.IPPROTO_TCP
	case unix.SOCK_DGRAM:
		socketMsg.Protocol = unix.IPPROTO_UDP
	default:
		return nil
	}

	msg.Socket = socketMsg
	return nil
}

func handleAccept(tracer *Tracer, _ *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	fd := tracer.ReadArgInt32(regs, 0)

	msg.Accept = &ebpfless.AcceptSyscallMsg{
		SocketFd: fd,
	}

	return nil
}

// Handle returns
func handleAcceptRet(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	addrlen, err := tracer.ReadArgInt32Ptr(process.Pid, regs, 2)
	if err != nil {
		return err
	}

	addr, err := parseAddrInfo(tracer, process, regs, addrlen)
	if err != nil {
		return err
	}

	m, ok := process.FdToSocket[msg.Accept.SocketFd]
	if !ok {
		return errors.New("unable to find socket")
	}

	msg.Type = ebpfless.SyscallTypeAccept

	msg.Accept = &ebpfless.AcceptSyscallMsg{
		MsgSocketInfo: ebpfless.MsgSocketInfo{
			AddressFamily: addr.af,
			Addr:          addr.ip,
			Port:          m.BoundToPort,
		},
	}

	return nil
}

func handleSocketRet(tracer *Tracer, process *Process, msg *ebpfless.SyscallMsg, regs syscall.PtraceRegs, _ bool) error {
	ret := int32(tracer.ReadRet(regs))

	if msg.Socket != nil && ret != -1 {
		process.FdToSocket[ret] = SocketInfo{
			AddressFamily: msg.Socket.AddressFamily,
			Protocol:      msg.Socket.Protocol,
		}
	}

	return nil
}

// Should send messages
func shouldSendConnect(msg *ebpfless.SyscallMsg) bool {
	return msg.Retval >= 0 || msg.Retval == -int64(syscall.EACCES) || msg.Retval == -int64(syscall.EPERM) || msg.Retval == -int64(syscall.ECONNREFUSED) || msg.Retval == -int64(syscall.ETIMEDOUT) || msg.Retval == -int64(syscall.EINPROGRESS)
}

func shouldSendAccept(msg *ebpfless.SyscallMsg) bool {
	return msg.Retval >= 0 || msg.Retval == -int64(syscall.EACCES) || msg.Retval == -int64(syscall.EPERM) || msg.Retval == -int64(syscall.ECONNABORTED)
}

func shouldSendBind(msg *ebpfless.SyscallMsg) bool {
	return msg.Retval >= 0 || msg.Retval == -int64(syscall.EACCES) || msg.Retval == -int64(syscall.EPERM) || msg.Retval == -int64(syscall.EADDRINUSE) || msg.Retval == -int64(syscall.EFAULT)
}
