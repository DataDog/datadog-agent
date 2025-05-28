// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package tests holds tests related files
package tests

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func TestSetSockOpt(t *testing.T) {
	SkipIfNotAvailable(t)

	ruleDefs := []*rules.RuleDefinition{
		{
			ID:         "test_rule_setsockopt",
			Expression: `setsockopt.socket != 0`,
		},
	}

	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	t.Run("setsockopt", func(t *testing.T) {
		var fd int
		defer func() {}()
		test.WaitSignal(t, func() error {
			var err error
			fd, err = syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_TCP)
			if err != nil {
				return err
			}
			defer syscall.Close(fd)

			return syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		}, func(event *model.Event, rule *rules.Rule) {
			assertTriggeredRule(t, rule, "test_rule_setsockopt")

			val, _ := event.GetFieldValue("setsockopt.socket")
			assert.Equal(t, int(fd), val, "setsockopt.socket mismatch")

			level, _ := event.GetFieldValue("setsockopt.level")
			assert.Equal(t, int(syscall.SOL_SOCKET), level)

			optname, _ := event.GetFieldValue("setsockopt.optname")
			assert.Equal(t, int(syscall.SO_REUSEADDR), optname)
		})
	})

}
