// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package provider

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	v2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakediscovery "k8s.io/client-go/discovery/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/autoscalinggate"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// newMinimalDPA returns the smallest valid DatadogPodAutoscaler accepted by NewFromKubernetes.
func newMinimalDPA(ns, name string) *datadoghq.DatadogPodAutoscaler {
	return &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec: datadoghq.DatadogPodAutoscalerSpec{
			Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
			TargetRef: v2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       name,
			},
		},
	}
}

// TestClusterBurstableDefaultIsBurstable tests the full chain from the config value
// through NewPodAutoscalerInternalBuilder (exactly as provider.go wires it) down to
// IsBurstable() on a DPA with no explicit spec.options.burstable or preview annotation.
//
// This catches accidental changes to the config default in common_settings.go.
func TestClusterBurstableDefaultIsBurstable(t *testing.T) {
	t.Run("default config (false) → IsBurstable returns false", func(t *testing.T) {
		cfgDefault := pkgconfigsetup.Datadog().GetBool("autoscaling.workload.options.burstable")
		assert.False(t, cfgDefault, "config default must be false")

		pai := model.NewPodAutoscalerInternalBuilder(cfgDefault).NewFromKubernetes(newMinimalDPA("ns", "app"))
		assert.False(t, pai.IsBurstable(),
			"IsBurstable() must be false when no spec/annotation overrides the cluster default")
	})

	t.Run("config overridden to true → IsBurstable returns true", func(t *testing.T) {
		pkgconfigsetup.Datadog().Set("autoscaling.workload.options.burstable", true, pkgconfigmodel.SourceUnknown)
		defer pkgconfigsetup.Datadog().Set("autoscaling.workload.options.burstable", false, pkgconfigmodel.SourceUnknown)

		cfgOverride := pkgconfigsetup.Datadog().GetBool("autoscaling.workload.options.burstable")
		pai := model.NewPodAutoscalerInternalBuilder(cfgOverride).NewFromKubernetes(newMinimalDPA("ns", "app"))
		assert.True(t, pai.IsBurstable(),
			"IsBurstable() must be true when DD_AUTOSCALING_WORKLOAD_OPTIONS_BURSTABLE=true")
	})
}

func TestIsArgoRolloutsAvailable(t *testing.T) {
	t.Run("returns true when rollouts resource exists", func(t *testing.T) {
		client := fakeclientset.NewClientset()
		fakeDiscovery := client.Discovery().(*fakediscovery.FakeDiscovery)
		fakeDiscovery.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "argoproj.io/v1alpha1",
				APIResources: []metav1.APIResource{
					{Kind: "Rollout", Name: "rollouts"},
					{Kind: "AnalysisTemplate", Name: "analysistemplates"},
				},
			},
		}

		assert.True(t, isArgoRolloutsAvailable(fakeDiscovery))
	})

	t.Run("returns false when group does not exist", func(t *testing.T) {
		client := fakeclientset.NewClientset()
		fakeDiscovery := client.Discovery().(*fakediscovery.FakeDiscovery)
		fakeDiscovery.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "apps/v1",
				APIResources: []metav1.APIResource{
					{Kind: "Deployment", Name: "deployments"},
				},
			},
		}

		assert.False(t, isArgoRolloutsAvailable(fakeDiscovery))
	})

	t.Run("returns false when group exists but rollouts resource is missing", func(t *testing.T) {
		client := fakeclientset.NewClientset()
		fakeDiscovery := client.Discovery().(*fakediscovery.FakeDiscovery)
		fakeDiscovery.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "argoproj.io/v1alpha1",
				APIResources: []metav1.APIResource{
					{Kind: "AnalysisTemplate", Name: "analysistemplates"},
				},
			},
		}

		assert.False(t, isArgoRolloutsAvailable(fakeDiscovery))
	})

	t.Run("returns false when no resources at all", func(t *testing.T) {
		client := fakeclientset.NewClientset()
		fakeDiscovery := client.Discovery().(*fakediscovery.FakeDiscovery)
		fakeDiscovery.Resources = []*metav1.APIResourceList{}

		assert.False(t, isArgoRolloutsAvailable(fakeDiscovery))
	})
}

func TestRunPodWatcherWhenReady(t *testing.T) {
	gate := autoscalinggate.New()
	ran := make(chan struct{})

	go runPodWatcherWhenReady(context.TODO(), gate, func(context.Context) {
		close(ran)
	})

	select {
	case <-ran:
		t.Fatal("run called before gate enabled")
	case <-time.After(50 * time.Millisecond):
	}

	gate.Enable()

	select {
	case <-ran:
		t.Fatal("run called before pod collection synced")
	case <-time.After(50 * time.Millisecond):
	}

	gate.MarkPodCollectionSynced()

	select {
	case <-ran:
	case <-time.After(time.Second):
		t.Fatal("run not called after gate enabled and synced")
	}
}
