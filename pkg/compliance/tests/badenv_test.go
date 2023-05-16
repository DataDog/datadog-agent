// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !docker && !linux && !kubeapiserver
// +build !docker,!linux,!kubeapiserver

package tests

import "testing"

func TestNoDocker(t *testing.T) {
	b := NewTestBench(t)
	defer b.Run()

	b.AddRule("NoDockerNoneScope").
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
		AssertNoEvent()

	b.AddRule("NoDockerWithScope").
		WithScope("docker").
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
		AssertNoEvent()
}

func TestNoKubernetes(t *testing.T) {
	b := NewTestBench(t)
	defer b.Run()

	b.AddRule("NoKubernetesNode").
		WithScope("kubernetesNode").
		WithInput(`
- kubeApiserver:
		kind: serviceaccounts
		version: v1
		fieldSelector: metadata.name=default
		apiRequest:
			verb: list
	type: array
	tag: serviceaccounts
`).
		WithRego(`
package datadog
import data.datadog as dd

has_key(o, k) {
	_ := o[k]
}

findings[f] {
	has_key(p, "serviceaccounts")
	f := dd.passed_finding(
		"my_resource_type",
		"my_resource_id",
		{"plop": input.context.hostname}
	)
}
`).
		AssertNoEvent()

	b.AddRule("NoKubernetesCluster").
		WithScope("kubernetesCluster").
		WithInput(`
- kubeApiserver:
		kind: serviceaccounts
		version: v1
		fieldSelector: metadata.name=default
		apiRequest:
			verb: list
	type: array
	tag: serviceaccounts
`).
		WithRego(`
package datadog
import data.datadog as dd

has_key(o, k) {
	_ := o[k]
}

findings[f] {
	has_key(p, "serviceaccounts")
	f := dd.passed_finding(
		"my_resource_type",
		"my_resource_id",
		{"plop": input.context.hostname}
	)
}
`).
		AssertNoEvent()
}
