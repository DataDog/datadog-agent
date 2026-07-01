// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario/scenarios/ec2host"
)

func TestDescribeCommandListsScenario(t *testing.T) {
	ec2host.Register()
	root := rootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"describe", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute describe: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "\"ec2-host\"") || !strings.Contains(s, "\"protocolVersion\": 1") {
		t.Fatalf("describe output missing scenario/protocol: %s", s)
	}
}

func TestCreateCommandHasSchemaFlags(t *testing.T) {
	ec2host.Register()
	root := rootCmd()
	// find: create ec2-host --help should list --os, --agent-version
	create, _, err := root.Find([]string{"create", "ec2-host"})
	if err != nil {
		t.Fatalf("find create ec2-host: %v", err)
	}
	for _, want := range []string{"os", "agent-version", "use-fakeintake"} {
		if create.Flags().Lookup(want) == nil {
			t.Errorf("create ec2-host missing --%s flag", want)
		}
	}
}
