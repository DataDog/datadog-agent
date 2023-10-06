// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/stretchr/testify/assert"
)

func TestProcessInput(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	self = filepath.Base(self)

	b := newTestBench(t)
	defer b.Run()

	b.AddRule("Self").
		WithInput(`
- process:
		name: %s
`, self).
		WithRego(`
package datadog
import data.datadog as dd
import data.helpers as h

has_key(o, k) {
	_ := o[k]
}

valid(p) {
	p.name = "tests.test"
	has_key(p, "cmdLine")
	has_key(p, "envs")
	has_key(p, "exe")
	has_key(p, "flags")
	has_key(p, "name")
	has_key(p, "pid")
}

findings[f] {
	valid(input.process[_])
	f := dd.passed_finding(
		"my_resource_type",
		"my_resource_id",
		{}
	)
}
`).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "Self", evt.RuleID)
			assert.Equal(t, 0, evt.RuleVersion)
			assert.Equal(t, "my_resource_id", evt.ResourceID)
			assert.Equal(t, "my_resource_type", evt.ResourceType)
			assert.Equal(t, compliance.RegoEvaluator, evt.Evaluator)
		})

	b.AddRule("SelfDuplicated").
		WithInput(`
- process:
    name: %s
  tag: self1
- process:
    name: %s
  tag: self2
`, self, self).
		WithRego(`
package datadog
import data.datadog as dd
import data.helpers as h

has_key(o, k) {
	_ := o[k]
}

valid(p) {
	p.name == "tests.test"
	has_key(p, "cmdLine")
	has_key(p, "envs")
	has_key(p, "exe")
	has_key(p, "flags")
	has_key(p, "name")
	has_key(p, "pid")
}

findings[f] {
	proc := input.self1[_]
	valid(proc)
	f := dd.passed_finding(
		"self1",
		"self1_id",
		{"name": proc.name},
	)
}

findings[f] {
	proc := input.self2[_]
	valid(proc)
	f := dd.passed_finding(
		"self2",
		"self2_id",
		{"name": proc.name},
	)
}
`).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "SelfDuplicated", evt.RuleID)
			assert.Equal(t, 0, evt.RuleVersion)
			assert.Equal(t, "self1_id", evt.ResourceID)
			assert.Equal(t, "self1", evt.ResourceType)
			assert.Equal(t, compliance.RegoEvaluator, evt.Evaluator)
			assert.Equal(t, self, evt.Data["name"])
		}).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "SelfDuplicated", evt.RuleID)
			assert.Equal(t, 0, evt.RuleVersion)
			assert.Equal(t, "self2_id", evt.ResourceID)
			assert.Equal(t, "self2", evt.ResourceType)
			assert.Equal(t, compliance.RegoEvaluator, evt.Evaluator)
			assert.Equal(t, self, evt.Data["name"])
		})

	b.AddRule("NoProcess").
		WithInput(`
- process:
		name: iprobablydonotexist
`).
		WithRego(`
package datadog
import data.datadog as dd
import data.helpers as h

findings[f] {
	input.process
	f := dd.passed_finding(
		"plop",
		"plop",
		{},
	)
}
`).
		AssertNoEvent()

	src := rand.NewSource(time.Now().UnixNano())
	rnd := rand.New(src)
	envFoo := rnd.Int()

	b.
		AddRule("Sleeps").
		Setup(func(t *testing.T, ctx context.Context) {
			cmd1 := exec.CommandContext(ctx, "sleep", "10")
			cmd2 := exec.CommandContext(ctx, "sleep", "10")
			cmd1.Env = []string{fmt.Sprintf("FOO=%d", envFoo)}
			cmd2.Env = []string{fmt.Sprintf("FOO=%d", envFoo)}
			if err := cmd1.Start(); err != nil {
				t.Fatal(err)
			}
			if err := cmd2.Start(); err != nil {
				t.Fatal(err)
			}
			// without calling Wait(), we may create zombie processes
			go cmd1.Wait()
			go cmd2.Wait()
		}).
		WithInput(`
- process:
		name: sleep
		envs:
			- FOO
			- BAR
`).
		WithRego(`
package datadog
import data.datadog as dd
import data.helpers as h

has_key(o, k) {
	_ := o[k]
}

valid(p) {
	p.name == "sleep"
	p.cmdLine[0] == "sleep"
	p.cmdLine[1] == "10"
	p.envs["FOO"] == "%d"
	not has_key(p.envs, "BAR")
}

findings[f] {
	c := count([p | p := input.process[_]; valid(p)])
	f := dd.passed_finding(
		"sleep",
		"sleep",
		{ "c": c },
	)
}
`, envFoo).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "sleep", evt.ResourceID)
			assert.Equal(t, "sleep", evt.ResourceType)
			c, _ := evt.Data["c"].(json.Number).Int64()
			// TODO(pierre): fix the flakyness of this test which sometimes returns 0 processes
			// on our CI.
			if c == 0 {
				t.Skip()
			} else {
				assert.Equal(t, int64(2), c)
			}
		})
}

func TestEtcGroup(t *testing.T) {
	b := newTestBench(t)
	defer b.Run()

	b.AddRule("EtcRootGroup").
		WithInput(`
- group:
		name: root
`).
		WithRego(`
package datadog
import data.datadog as dd
import data.helpers as h

findings[f] {
	input.group.id == 0
	input.group.name == "root"
	_ := input.group.users
	f := dd.passed_finding(
		"group_id",
		"group_type",
		{},
	)
}
`).
		AssertPassedEvent(nil)

	b.AddRule("EtcGroupNotExist").
		WithInput(`
- group:
		name: asdasdasdas
`).
		WithRego(`
package datadog
import data.datadog as dd
import data.helpers as h

has_key(o, k) {
	_ := o[k]
}

findings[f] {
	not has_key(input, "group")
	f := dd.passed_finding(
		"group_id",
		"group_type",
		{},
	)
}
`).
		AssertPassedEvent(nil)
}
