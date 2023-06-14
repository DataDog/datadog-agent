// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"sort"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/assert"
)

func TestProcessArgsFlags(t *testing.T) {
	var argsEntry model.ArgsEntry
	argsEntry.Values = []string{
		"cmd", "-abc", "--verbose", "test",
		"-v=1", "--host=myhost",
		"-9", "-", "--",
	}

	resolver, _ := process.NewResolver(&manager.Manager{}, &config.Config{}, &statsd.NoOpClient{},
		&procutil.DataScrubber{}, nil, nil, nil, nil, nil, nil, process.NewResolverOpts())

	e := model.Event{
		Exec: model.ExecEvent{
			Process: &model.Process{
				ArgsEntry: &argsEntry,
			},
		},
		FieldHandlers: &FieldHandlers{
			resolvers: &resolvers.Resolvers{
				ProcessResolver: resolver,
			},
		},
	}

	flags := e.FieldHandlers.ResolveProcessArgsFlags(&e, e.Exec.Process)
	sort.Strings(flags)

	hasFlag := func(flags []string, flag string) bool {
		i := sort.SearchStrings(flags, flag)
		return i < len(flags) && flags[i] == flag
	}

	if !hasFlag(flags, "a") {
		t.Error("flags 'a' not found")
	}

	if !hasFlag(flags, "b") {
		t.Error("flags 'b' not found")
	}

	if hasFlag(flags, "v") {
		t.Error("flags 'v' found")
	}

	if !hasFlag(flags, "9") {
		t.Error("flags '9' not found found")
	}

	if !hasFlag(flags, "abc") {
		t.Error("flags 'abc' not found")
	}

	if !hasFlag(flags, "verbose") {
		t.Error("flags 'verbose' not found")
	}

	if len(flags) != 6 {
		t.Errorf("expected 6 flags, got %d", len(flags))
	}
}

func TestProcessArgsOptions(t *testing.T) {
	var argsEntry model.ArgsEntry
	argsEntry.Values = []string{
		"cmd", "--config", "/etc/myfile", "--host=myhost", "--verbose",
		"-c", "/etc/myfile", "-e", "", "-h=myhost", "-v",
		"--", "---", "-9",
	}

	resolver, _ := process.NewResolver(&manager.Manager{}, &config.Config{}, &statsd.NoOpClient{},
		&procutil.DataScrubber{}, nil, nil, nil, nil, nil, nil, process.NewResolverOpts())

	e := model.Event{
		Exec: model.ExecEvent{
			Process: &model.Process{
				ArgsEntry: &argsEntry,
			},
		},
		FieldHandlers: &FieldHandlers{
			resolvers: &resolvers.Resolvers{
				ProcessResolver: resolver,
			},
		},
	}

	options := e.FieldHandlers.ResolveProcessArgsOptions(&e, e.Exec.Process)
	sort.Strings(options)

	hasOption := func(options []string, option string) bool {
		i := sort.SearchStrings(options, option)
		return i < len(options) && options[i] == option
	}

	if !hasOption(options, "config=/etc/myfile") {
		t.Error("option 'config=/etc/myfile' not found")
	}

	if !hasOption(options, "c=/etc/myfile") {
		t.Error("option 'c=/etc/myfile' not found")
	}

	if !hasOption(options, "e=") {
		t.Error("option 'e=' not found")
	}

	if !hasOption(options, "host=myhost") {
		t.Error("option 'host=myhost' not found")
	}

	if !hasOption(options, "h=myhost") {
		t.Error("option 'h=myhost' not found")
	}

	if hasOption(options, "verbose=") {
		t.Error("option 'verbose=' found")
	}

	if len(options) != 5 {
		t.Errorf("expected 5 options, got %d", len(options))
	}
}

func TestBestGuessServiceValues(t *testing.T) {

	type testEntry struct {
		name     string
		values   []string
		expected string
	}

	entries := []testEntry{
		{
			name: "basic",
			values: []string{
				"datadog-agent",
				"d",
				"datadog-a",
			},
			expected: "datadog-agent",
		},
		{
			name: "single",
			values: []string{
				"aa",
			},
			expected: "aa",
		},
		{
			name:     "empty",
			values:   []string{},
			expected: "",
		},
		{
			name: "divergent",
			values: []string{
				"aa",
				"bb",
			},
			expected: "aa",
		},
		{
			name: "divergent-2",
			values: []string{
				"bb",
				"aa",
			},
			expected: "bb",
		},
	}

	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			assert.Equal(t, entry.expected, bestGuessServiceTag(entry.values))
		})
	}
}
