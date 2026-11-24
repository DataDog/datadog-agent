// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package checktest

import (
	"slices"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

// contentObjects are the objects whose payload content the tests compare: every built-in resource and the cluster,
// and two built-in custom resources. The built-in custom resources share their collector and processor, so a
// resource-specific one and a group-wide one cover them.
func contentObjects(t *testing.T) []string {
	return append(suite.Live(suite.BuiltinUIDs(t)...), "live cluster",
		"live "+suite.CustomResourceUID("argoproj.io/v1alpha1/rollouts"),
		"live "+suite.CustomResourceUID("karpenter.sh/v1/samples"))
}

// firstRun are the objects that the check sends at its first run in a cluster with the objects: each of them, and
// the cluster, as the nodes changed.
func firstRun(uids []string) []string {
	return append(suite.Live(uids...), "live cluster")
}

// deletion are the objects that the check sends when the objects are deleted. Known defect: the cluster collector
// starts the node informer first, which only calls its delete handler, so the check does not report deleted nodes.
func deletion(uids []string) []string {
	return suite.Deleted(slices.DeleteFunc(slices.Clone(uids), func(uid string) bool { return strings.HasPrefix(uid, "v1_nodes") })...)
}

func except(uids []string, excluded ...string) []string {
	return slices.DeleteFunc(slices.Clone(uids), func(uid string) bool { return slices.Contains(excluded, uid) })
}

func withPrefix(uids []string, prefix string) []string {
	return slices.DeleteFunc(slices.Clone(uids), func(uid string) bool { return !strings.HasPrefix(uid, prefix) })
}
