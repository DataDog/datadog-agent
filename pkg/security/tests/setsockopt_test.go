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
			ID:         "test_rule_setsockopt",
			Expression: `setsockopt.level == SOL_SOCKET && setsockopt.optname == SO_ATTACH_FILTER`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("setsockopt", func(t *testing.T) {
		var fd int
		type SockFilter struct {
			Code uint16
			Jt   uint8
			Jf   uint8
			K    uint32
		}

		type SockFprog struct {
			Len    uint16
			_      [6]byte
			Filter *SockFilter
		}

		test.WaitSignal(t, func() error {
			var err error
			fd, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_TCP)
			if err != nil {
				return err
			}
			defer syscall.Close(fd)

			filter := []SockFilter{
				{Code: 0x06, Jt: 0, Jf: 0, K: 0xFFFFFFFF}, // BPF_RET | BPF_K
			}
			prog := SockFprog{
				Len:    uint16(len(filter)),
				Filter: &filter[0],
			}

			_, _, errno := syscall.Syscall6(
				syscall.SYS_SETSOCKOPT,
				uintptr(fd),
				uintptr(syscall.SOL_SOCKET),
				uintptr(syscall.SO_ATTACH_FILTER),
				uintptr(unsafe.Pointer(&prog)),
				uintptr(unsafe.Sizeof(prog)),
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

}
