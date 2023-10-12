// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package tests

import (
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"

	"github.com/stretchr/testify/assert"
)

func TestPackageDpkg(t *testing.T) {
	if _, err := os.Stat("/var/lib/dpkg/status"); err != nil {
		t.Skip()
	}

	b := newTestBench(t)
	defer b.Run()

	b.AddRule("PackageExists").
		WithInput(`
- package:
		names:
			- libc6
	tag: libc
`).
		WithRego(`
package datadog
import data.datadog as dd
import data.helpers as h

has_key(o, k) {
	_ := o[k]
}

findings[f] {
	has_key(input, "libc")
	input.libc.name == "libc6"
	has_key(input.libc, "version")
	has_key(input.libc, "arch")
	f := dd.passed_finding(
		"my_resource_type",
		"my_resource_id",
		{"foo": "bar"}
	)
}
`).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "my_resource_id", evt.ResourceID)
			assert.Equal(t, "my_resource_type", evt.ResourceType)
			assert.Equal(t, "bar", evt.Data["foo"])
		})
}

func TestPackageApk(t *testing.T) {
	if _, err := os.Stat("/lib/apk/db/installed"); err != nil {
		t.Skip()
	}

	b := newTestBench(t)
	defer b.Run()

	b.AddRule("PackageExists").
		WithInput(`
- package:
		names:
			- libc-utils
	tag: libc
`).
		WithRego(`
package datadog
import data.datadog as dd
import data.helpers as h

has_key(o, k) {
	_ := o[k]
}

findings[f] {
	has_key(input, "libc")
	input.libc.name == "libc-utils"
	has_key(input.libc, "version")
	has_key(input.libc, "arch")
	f := dd.passed_finding(
		"my_resource_type",
		"my_resource_id",
		{"foo": "bar"}
	)
}
`).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "my_resource_id", evt.ResourceID)
			assert.Equal(t, "my_resource_type", evt.ResourceType)
			assert.Equal(t, "bar", evt.Data["foo"])
		})
}
