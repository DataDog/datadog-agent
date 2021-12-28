// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build functionaltests

package tests

import (
	"syscall"
	"testing"

	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/stretchr/testify/assert"
)

func BenchmarkSerializers(b *testing.B) {
	// Let's first fetch a realistic event

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/test-open" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(b, nil, []*rules.RuleDefinition{rule}, testOpts{})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	_, testFilePtr, err := test.Path("test-open")
	if err != nil {
		b.Fatal(err)
	}

	var workingEvent *sprobe.Event
	test.WaitSignal(b, func() error {
		fd, _, errno := syscall.Syscall6(syscall.SYS_OPENAT, 0, uintptr(testFilePtr), syscall.O_CREAT, 0711, 0, 0)
		if errno != 0 {
			return error(errno)
		}
		return syscall.Close(int(fd))
	}, func(event *sprobe.Event, r *rules.Rule) {
		workingEvent = event
		assert.Equal(b, "open", event.GetType(), "wrong event type")
	})

	// then we run the benchmark
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := workingEvent.MarshalJSON()
		if err != nil {
			b.Error(err)
		}
	}
}
