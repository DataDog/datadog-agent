// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"fmt"
	"syscall"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSetSockOpt(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID: "test_rule_setsockopt",
			Expression: `setsockopt.level == SOL_SOCKET 
			&& setsockopt.optname == SO_ATTACH_FILTER 
			&& setsockopt.socket_type == SOCK_RAW 
			&& setsockopt.socket_protocol == 6 
			&& setsockopt.socket_family == AF_INET 
			&& setsockopt.filter_hash == "627019f67a3853590209488302dd51282834c4f9f9c1cc43274f45c4bfd9869f"`,
		},
		{
			ID: "test_rule_setsockopt_udp",
			Expression: `setsockopt.level == SOL_SOCKET 
			&& setsockopt.optname == SO_ATTACH_FILTER 
			&& setsockopt.socket_type == SOCK_DGRAM 
			&& setsockopt.socket_protocol == 17 
			&& setsockopt.socket_family == AF_INET 
			&& setsockopt.filter_hash == "627019f67a3853590209488302dd51282834c4f9f9c1cc43274f45c4bfd9869f"`,
		},
		{
			ID: "test_rule_setsockopt_tcp",
			Expression: `setsockopt.level == SOL_SOCKET 
			&& setsockopt.optname == SO_ATTACH_FILTER 
			&& setsockopt.socket_type == SOCK_STREAM 
			&& setsockopt.socket_protocol == 6 
			&& setsockopt.socket_family == AF_INET 
			&& setsockopt.filter_hash == "627019f67a3853590209488302dd51282834c4f9f9c1cc43274f45c4bfd9869f"`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("setsockopt", func(t *testing.T) {
		var fd int

		test.WaitSignal(t, func() error {
			var err error
			fd, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_TCP)
			if err != nil {
				return err
			}
			defer syscall.Close(fd)

			// BPF Filter
			program := []syscall.SockFilter{
				{Code: 0x28, Jt: 0, Jf: 0, K: 0x0000000c},
				{Code: 0x15, Jt: 0, Jf: 8, K: 0x000086dd},
				{Code: 0x30, Jt: 0, Jf: 0, K: 0x00000014},
				{Code: 0x15, Jt: 2, Jf: 0, K: 0x00000084},
				{Code: 0x15, Jt: 1, Jf: 0, K: 0x00000006},
				{Code: 0x15, Jt: 0, Jf: 17, K: 0x00000011},
				{Code: 0x28, Jt: 0, Jf: 0, K: 0x00000036},
				{Code: 0x15, Jt: 14, Jf: 0, K: 0x00000016},
				{Code: 0x28, Jt: 0, Jf: 0, K: 0x00000038},
				{Code: 0x15, Jt: 12, Jf: 13, K: 0x00000016},
				{Code: 0x15, Jt: 0, Jf: 12, K: 0x00000800},
				{Code: 0x30, Jt: 0, Jf: 0, K: 0x00000017},
				{Code: 0x15, Jt: 2, Jf: 0, K: 0x00000084},
				{Code: 0x15, Jt: 1, Jf: 0, K: 0x00000006},
				{Code: 0x15, Jt: 0, Jf: 8, K: 0x00000011},
				{Code: 0x28, Jt: 0, Jf: 0, K: 0x00000014},
				{Code: 0x45, Jt: 6, Jf: 0, K: 0x00001fff},
				{Code: 0xb1, Jt: 0, Jf: 0, K: 0x0000000e},
				{Code: 0x48, Jt: 0, Jf: 0, K: 0x0000000e},
				{Code: 0x15, Jt: 2, Jf: 0, K: 0x00000016},
				{Code: 0x48, Jt: 0, Jf: 0, K: 0x00000010},
				{Code: 0x15, Jt: 0, Jf: 1, K: 0x00000016},
				{Code: 0x06, Jt: 0, Jf: 0, K: 0x0000ffff},
				{Code: 0x06, Jt: 0, Jf: 0, K: 0x00000000},
			}

			// Create structure
			filter := syscall.SockFprog{
				Len:    uint16(len(program)),
				Filter: &program[0],
			}
			_, _, errno := syscall.Syscall6(
				syscall.SYS_SETSOCKOPT,
				uintptr(fd),
				uintptr(syscall.SOL_SOCKET),
				uintptr(syscall.SO_ATTACH_FILTER),
				uintptr(unsafe.Pointer(&filter)),
				uintptr(unsafe.Sizeof(filter)),
				0,
			)

			if errno != 0 {
				return fmt.Errorf("setsockopt failed: %v", errno)
			}

			return nil
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_setsockopt")
		})
	})

	t.Run("setsockopt-DGRAM-socket", func(t *testing.T) {
		var fd int

		defer func() {}()

		test.WaitSignal(t, func() error {
			var err error
			fd, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
			if err != nil {
				return err
			}
			defer syscall.Close(fd)

			// BPF Filter
			program := []syscall.SockFilter{
				{Code: 0x28, Jt: 0, Jf: 0, K: 0x0000000c},
				{Code: 0x15, Jt: 0, Jf: 8, K: 0x000086dd},
				{Code: 0x30, Jt: 0, Jf: 0, K: 0x00000014},
				{Code: 0x15, Jt: 2, Jf: 0, K: 0x00000084},
				{Code: 0x15, Jt: 1, Jf: 0, K: 0x00000006},
				{Code: 0x15, Jt: 0, Jf: 17, K: 0x00000011},
				{Code: 0x28, Jt: 0, Jf: 0, K: 0x00000036},
				{Code: 0x15, Jt: 14, Jf: 0, K: 0x00000016},
				{Code: 0x28, Jt: 0, Jf: 0, K: 0x00000038},
				{Code: 0x15, Jt: 12, Jf: 13, K: 0x00000016},
				{Code: 0x15, Jt: 0, Jf: 12, K: 0x00000800},
				{Code: 0x30, Jt: 0, Jf: 0, K: 0x00000017},
				{Code: 0x15, Jt: 2, Jf: 0, K: 0x00000084},
				{Code: 0x15, Jt: 1, Jf: 0, K: 0x00000006},
				{Code: 0x15, Jt: 0, Jf: 8, K: 0x00000011},
				{Code: 0x28, Jt: 0, Jf: 0, K: 0x00000014},
				{Code: 0x45, Jt: 6, Jf: 0, K: 0x00001fff},
				{Code: 0xb1, Jt: 0, Jf: 0, K: 0x0000000e},
				{Code: 0x48, Jt: 0, Jf: 0, K: 0x0000000e},
				{Code: 0x15, Jt: 2, Jf: 0, K: 0x00000016},
				{Code: 0x48, Jt: 0, Jf: 0, K: 0x00000010},
				{Code: 0x15, Jt: 0, Jf: 1, K: 0x00000016},
				{Code: 0x06, Jt: 0, Jf: 0, K: 0x0000ffff},
				{Code: 0x06, Jt: 0, Jf: 0, K: 0x00000000},
			}

			// Create structure
			filter := syscall.SockFprog{
				Len:    uint16(len(program)),
				Filter: &program[0],
			}
			_, _, errno := syscall.Syscall6(
				syscall.SYS_SETSOCKOPT,
				uintptr(fd),
				uintptr(syscall.SOL_SOCKET),
				uintptr(syscall.SO_ATTACH_FILTER),
				uintptr(unsafe.Pointer(&filter)),
				uintptr(unsafe.Sizeof(filter)),
				0,
			)

			if errno != 0 {
				return fmt.Errorf("setsockopt failed: %v", errno)
			}

			return nil
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_setsockopt_udp")
		})
	})

	t.Run("setsockopt-STREAM-socket", func(t *testing.T) {
		var fd int

		defer func() {}()

		test.WaitSignal(t, func() error {
			var err error
			fd, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
			if err != nil {
				return err
			}
			defer syscall.Close(fd)

			// BPF Filter
			program := []syscall.SockFilter{
				{Code: 0x28, Jt: 0, Jf: 0, K: 0x0000000c},
				{Code: 0x15, Jt: 0, Jf: 8, K: 0x000086dd},
				{Code: 0x30, Jt: 0, Jf: 0, K: 0x00000014},
				{Code: 0x15, Jt: 2, Jf: 0, K: 0x00000084},
				{Code: 0x15, Jt: 1, Jf: 0, K: 0x00000006},
				{Code: 0x15, Jt: 0, Jf: 17, K: 0x00000011},
				{Code: 0x28, Jt: 0, Jf: 0, K: 0x00000036},
				{Code: 0x15, Jt: 14, Jf: 0, K: 0x00000016},
				{Code: 0x28, Jt: 0, Jf: 0, K: 0x00000038},
				{Code: 0x15, Jt: 12, Jf: 13, K: 0x00000016},
				{Code: 0x15, Jt: 0, Jf: 12, K: 0x00000800},
				{Code: 0x30, Jt: 0, Jf: 0, K: 0x00000017},
				{Code: 0x15, Jt: 2, Jf: 0, K: 0x00000084},
				{Code: 0x15, Jt: 1, Jf: 0, K: 0x00000006},
				{Code: 0x15, Jt: 0, Jf: 8, K: 0x00000011},
				{Code: 0x28, Jt: 0, Jf: 0, K: 0x00000014},
				{Code: 0x45, Jt: 6, Jf: 0, K: 0x00001fff},
				{Code: 0xb1, Jt: 0, Jf: 0, K: 0x0000000e},
				{Code: 0x48, Jt: 0, Jf: 0, K: 0x0000000e},
				{Code: 0x15, Jt: 2, Jf: 0, K: 0x00000016},
				{Code: 0x48, Jt: 0, Jf: 0, K: 0x00000010},
				{Code: 0x15, Jt: 0, Jf: 1, K: 0x00000016},
				{Code: 0x06, Jt: 0, Jf: 0, K: 0x0000ffff},
				{Code: 0x06, Jt: 0, Jf: 0, K: 0x00000000},
			}

			// Create structure
			filter := syscall.SockFprog{
				Len:    uint16(len(program)),
				Filter: &program[0],
			}
			_, _, errno := syscall.Syscall6(
				syscall.SYS_SETSOCKOPT,
				uintptr(fd),
				uintptr(syscall.SOL_SOCKET),
				uintptr(syscall.SO_ATTACH_FILTER),
				uintptr(unsafe.Pointer(&filter)),
				uintptr(unsafe.Sizeof(filter)),
				0,
			)

			if errno != 0 {
				return fmt.Errorf("setsockopt failed: %v", errno)
			}

			return nil
		}, func(_ *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_setsockopt_tcp")
		})
	})

}
