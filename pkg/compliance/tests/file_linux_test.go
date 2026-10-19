// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tests

import (
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"

	"github.com/stretchr/testify/assert"
)

func TestFileSpecial(t *testing.T) {
	b := newTestBench(t)
	defer b.Run()

	// A socket path must fit in sun_path, so keep the directory short.
	dir, err := os.MkdirTemp("", "cmp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name, path, parser string
	}{
		{"Socket", sock, ""},
		{"SocketRaw", sock, "raw"},
		{"FIFO", fifo, ""},
		{"FIFOJSON", fifo, "json"},
	} {
		input := `
- file:
		path: %s
`
		if c.parser != "" {
			input += "\t\tparser: " + c.parser + "\n"
		}
		b.AddRule(c.name).
			WithInput(input, c.path).
			WithRego(`
package datadog
import data.datadog as dd

findings[f] {
	input.file.path == %q
	input.file.content == null
	_ := input.file.permissions
	_ := input.file.user
	_ := input.file.group
	f := dd.passed_finding(
		"foo",
		"bar",
		{"permissions": input.file.permissions}
	)
}
`, c.path).
			AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
				assert.NotZero(t, evt.Data["permissions"])
			})
	}
}
