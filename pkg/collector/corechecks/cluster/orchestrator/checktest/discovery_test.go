// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package checktest

import (
	"errors"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

func TestDiscovery(t *testing.T) {
	t.Run("failure_at_start_recovers", func(t *testing.T) {
		env := suite.Setup(t, suite.WithDiscoveryFailure(errors.New("the API server is unavailable")))

		suite.AssertObjects(t, nil, env.Pop(t))
		env.FailDiscovery(nil)
		suite.AssertObjects(t, firstRun(env.UIDs()), env.Pop(t, firstRun(env.UIDs())...))
	})

	// Discovery fails for one API group only, like an unavailable aggregated API.
	t.Run("partial_failure_recovers", func(t *testing.T) {
		env := suite.Setup(t, suite.WithFailedGroupVersions("argoproj.io/v1alpha1"))
		argo := withPrefix(env.UIDs(), "argoproj.io_v1alpha1_")

		suite.AssertObjects(t, firstRun(except(env.UIDs(), argo...)), env.Pop(t, firstRun(except(env.UIDs(), argo...))...))
		env.FailGroupVersions()
		suite.AssertObjects(t, suite.Live(argo...), env.Pop(t, suite.Live(argo...)...))
	})

	// The check collects the custom resources whose CRDs are installed after it started.
	t.Run("crds_installed_after_start", func(t *testing.T) {
		env := suite.Setup(t, suite.WithUndiscoveredCustomResources())
		customResources := env.CustomResourceUIDs()

		suite.AssertObjects(t, firstRun(except(env.UIDs(), customResources...)), env.Pop(t, firstRun(except(env.UIDs(), customResources...))...))
		env.InstallCRDs(t)
		expected := append(suite.Live(customResources...), suite.Live(crdUIDs(customResources)...)...)
		suite.AssertObjects(t, expected, env.Pop(t, expected...))
	})

	// The API server reports a custom resource once its CRD is established, which a CRD update can do after its
	// creation.
	t.Run("crds_established_after_their_creation", func(t *testing.T) {
		env := suite.Setup(t, suite.WithUndiscoveredCustomResources())
		customResources := env.CustomResourceUIDs()

		suite.AssertObjects(t, firstRun(except(env.UIDs(), customResources...)), env.Pop(t, firstRun(except(env.UIDs(), customResources...))...))
		for _, uid := range customResources {
			env.Add(t, suite.CRD(t, env.Object(t, uid), false))
		}
		suite.AssertObjects(t, suite.Live(crdUIDs(customResources)...), env.Pop(t, suite.Live(crdUIDs(customResources)...)...))
		for _, uid := range customResources {
			env.Discover(t, env.Object(t, uid))
			env.Update(t, suite.Updated(t, suite.CRD(t, env.Object(t, uid), true)))
		}
		expected := append(suite.Live(customResources...), suite.Live(crdUIDs(customResources)...)...)
		suite.AssertObjects(t, expected, env.Pop(t, expected...))
	})

	t.Run("crd_collectors_only", func(t *testing.T) {
		env := suite.Setup(t, suite.WithCRDCollectorsOnly())

		suite.AssertObjects(t, suite.Live(env.CustomResourceUIDs()...), env.Pop(t, suite.Live(env.CustomResourceUIDs()...)...))
	})

	// The check keeps the version of a resource that it selected first, even when its CRD then serves a preferred one.
	// The version merge is the same for every resource, so one custom resource covers it.
	t.Run("crd_serves_a_new_version", func(t *testing.T) {
		older := suite.CustomResource(t, "gateway.networking.k8s.io/v1beta1/gateways")
		crd := suite.CRD(t, older, true)
		env := suite.Setup(t, suite.WithCollectors("customresourcedefinitions"), suite.WithObjects(older, crd))
		expected := []string{"live gateway.networking.k8s.io_v1beta1_gateways", "live gateway.networking.k8s.io_v1beta1_gateways_crd"}
		suite.AssertObjects(t, expected, env.Pop(t, expected...))

		// The API server serves the same object in each version of its CRD. The new resource version shows a payload
		// of the new version, which the cache would otherwise skip.
		newer := suite.Updated(t, suite.Named(t, suite.CustomResource(t, "gateway.networking.k8s.io/v1/gateways"), "gateway.networking.k8s.io_v1beta1_gateways"))
		env.Add(t, newer)
		env.Discover(t, newer)
		crd = suite.Updated(t, crd).(*apiextensionsv1.CustomResourceDefinition)
		crd.Spec.Versions = append([]apiextensionsv1.CustomResourceDefinitionVersion{{Name: "v1", Served: true, Storage: true}}, crd.Spec.Versions[0])
		crd.Spec.Versions[1].Storage = false
		env.Update(t, crd)
		suite.AssertObjects(t, []string{"live gateway.networking.k8s.io_v1beta1_gateways_crd"}, env.Pop(t, "live gateway.networking.k8s.io_v1beta1_gateways_crd"))
	})
}

func crdUIDs(customResources []string) []string {
	uids := make([]string, 0, len(customResources))
	for _, uid := range customResources {
		uids = append(uids, uid+"_crd")
	}
	return uids
}
