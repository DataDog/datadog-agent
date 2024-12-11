// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package tests

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/compliance"

	"github.com/DataDog/datadog-agent/pkg/util/docker"
	"github.com/stretchr/testify/assert"
)

const dockerHostname = "host-docker-test-42"

func TestDockerInfoInput(t *testing.T) {
	dockerCl, err := docker.ConnectToDocker(context.Background())
	if err != nil {
		t.Skipf("could not connect to docker to start testing: %v", err)
	}

	b := newTestBench(t).
		WithHostname(dockerHostname).
		WithDockerClient(dockerCl)
	defer b.Run()

	b.AddRule("TaggedInfos").
		WithInput(`
- docker:
    kind: info
  type: array
  tag: infos
`).
		WithRego(`
package datadog
import data.datadog as dd

has_key(o, k) {
	_ := o[k]
}

valid_info(p) {
	has_key(p, "inspect")
}

valid_infos = [i | i := input.infos[_]; valid_info(i)]

findings[f] {
	count(input.infos) > 0
	count(valid_infos) == count(input.infos)
	f := dd.passed_finding(
		"my_resource_type",
		"my_resource_id",
		{"plop": input.context.hostname}
	)
}
`).
		AssertPassedEvent(func(t *testing.T, e *compliance.CheckEvent) {
			assert.Equal(t, "TaggedInfos", e.RuleID)
			assert.Equal(t, "my_resource_type", e.ResourceType)
			assert.Equal(t, "my_resource_id", e.ResourceID)
			assert.Equal(t, map[string]interface{}{"plop": dockerHostname}, e.Data)
		})

	b.AddRule("RegoContext").
		WithInput(`
- docker:
    kind: info
  type: array
  tag: infos
`).
		WithRego(`
package datadog
import data.datadog as dd

has_key(o, k) {
	_ := o[k]
}

valid_context(ctx) {
	ctx.hostname == "{{.Hostname}}"
	ctx.ruleID == "{{.RuleID}}"
	ctx.input.infos.docker.kind == "info"
	ctx.input.infos.type == "array"
}

findings[f] {
	valid_context(input.context)
	f := dd.passed_finding(
		"valid_context",
		"valid_context_id",
		{"foo": "bar"}
	)
}
`).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.Equal(t, "valid_context", evt.ResourceType)
			assert.Equal(t, "valid_context_id", evt.ResourceID)
			assert.Equal(t, map[string]interface{}{"foo": "bar"}, evt.Data)
		})

	b.
		AddRule("UnknownKind").
		WithInput(`
- docker:
		kind: plop
	type: array
	tag: plop
`).
		WithRego(`package datadog`).
		AssertErrorEvent()

	b.
		AddRule("TaggedImages").
		WithInput(`
- docker:
		kind: image
	type: array
	tag: dockerimages
`).
		WithRego(`
package datadog
import data.datadog as dd

has_key(o, k) {
	_ := o[k]
}

valid_image(i) {
	has_key(i, "id")
	has_key(i, "inspect")
	has_key(i.inspect, "Architecture")
	has_key(i.inspect, "Comment")
	has_key(i.inspect, "Config")
}

valid_images = [i | i := input.dockerimages[_]; valid_image(i)]

findings[f] {
	count(input.dockerimages) > 0
	count(valid_images) == count(input.dockerimages)
	f := dd.passed_finding(
		"my_resource_type",
		"my_resource_id",
		{"ids": [i.id | i := input.dockerimages[_]]}
	)
}
`).
		AssertPassedEvent(func(t *testing.T, evt *compliance.CheckEvent) {
			assert.NotEmpty(t, evt.Data["ids"])
		})

	b.
		AddRule("Network").
		WithInput(`
- docker:
		kind: network
	type: array
	tag: nets
`).
		WithRego(`
package datadog
import data.datadog as dd

findings[f] {
	count(input.nets) > 0
	_ := input.nets[0]["id"]
	_ := input.nets[0]["inspect"]
	input.nets[0].inspect
	f := dd.passed_finding(
		"net_resource",
		"net_resource_id",
		{},
	)
}
`).
		AssertPassedEvent(nil)

	b.AddRule("Version").
		WithInput(`
- docker:
		kind: version
	type: array
	tag: version
`).
		WithRego(`
package datadog
import data.datadog as dd

findings[f] {
	version := input.version[_]
	_ := version.apiVersion
	_ := version.arch
	_ := version.kernelVersion
	_ := version.os
	_ := version.platform
	_ := version.version
	f := dd.passed_finding(
		"version_resource",
		"version_resource_id",
		{},
	)
}
`).
		AssertPassedEvent(nil)
}
