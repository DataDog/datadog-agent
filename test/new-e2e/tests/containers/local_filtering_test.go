// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file adds entry points that run the ORIGINAL containers filtering
// suites (k8sFilteringSuiteBase, the same bodies TestK8SLegacyFilteringSuite
// and TestK8SCELFilteringSuite run on Pulumi-provisioned kind-on-EC2
// environments) against a local e2ectl kind environment — the same test
// bodies, only the provisioning differs. The environments come from
// e2ectl-kind-legacy-filtering.yml and e2ectl-kind-cel-filtering.yml next to
// this file (see their headers for the exact commands).
//
// This is the migration pattern: the suites and their assertions are
// unchanged; the environment is described by a config file next to the test
// instead of Pulumi scenario options (the embedded exclude fixtures become
// agent.helm.values; the K8sAppDefinition topology becomes workloads:
// entries), and the entry points attach to a live environment instead of
// provisioning one.
package containers

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	e2ectlenv "github.com/DataDog/datadog-agent/test/new-e2e/utils/e2ectlenv"
)

// localKindLegacyFilteringSuite runs the existing legacy filtering suite
// (containerExclude patterns) against a local e2ectl kind environment.
type localKindLegacyFilteringSuite struct {
	k8sFilteringSuiteBase
}

// TestK8SLegacyFilteringSuiteOnLocalKind runs the ORIGINAL legacy filtering
// suite against a local e2ectl kind environment started from
// e2ectl-kind-legacy-filtering.yml. The workload topology is the suite's
// Pulumi one: nginx in workload-nginx, redis in default, nginx in
// filtered-ns (see the config's workloads section for the exact mapping).
func TestK8SLegacyFilteringSuiteOnLocalKind(t *testing.T) {
	envName := e2ectlenv.RequireEnv(t)
	e2ectlenv.RequireSnapshot(t, envName)
	t.Parallel()
	e2e.Run(t, &localKindLegacyFilteringSuite{}, e2e.WithProvisioner(
		e2ectlenv.Attach[environments.Kubernetes](envName),
	))
}

// localKindCELFilteringSuite runs the existing CEL filtering suite
// (DD_CEL_WORKLOAD_EXCLUDE rules) against a local e2ectl kind environment.
type localKindCELFilteringSuite struct {
	k8sFilteringSuiteBase
}

// TestK8SCELFilteringSuiteOnLocalKind runs the ORIGINAL CEL filtering suite
// against a local e2ectl kind environment started from
// e2ectl-kind-cel-filtering.yml — the same workload topology as the legacy
// entry, with the CEL exclude rules instead of containerExclude patterns.
func TestK8SCELFilteringSuiteOnLocalKind(t *testing.T) {
	envName := e2ectlenv.RequireEnv(t)
	e2ectlenv.RequireSnapshot(t, envName)
	t.Parallel()
	e2e.Run(t, &localKindCELFilteringSuite{}, e2e.WithProvisioner(
		e2ectlenv.Attach[environments.Kubernetes](envName),
	))
}
