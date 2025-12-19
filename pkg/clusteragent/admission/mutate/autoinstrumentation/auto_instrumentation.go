// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package autoinstrumentation implements the webhook that injects APM libraries into pods. It is the mutating webhook
// for the Single Step Instrumentation product feature in Kubernetes.
package autoinstrumentation

import (
	"fmt"
	"time"

	"k8s.io/client-go/dynamic"
	dynamic_informer "k8s.io/client-go/dynamic/dynamicinformer"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	configWebhook "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/tagsfromlabels"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/servicemonitor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// NewAutoInstrumentation is a helper function to create a fully initialized webhook for SSI. Our webhook is made up of
// several components, but consumers of this webhook should not need to care about how the webhook is wired together.
func NewAutoInstrumentation(datadogConfig config.Component, wmeta workloadmeta.Component, rcClient *rcclient.Client) (*Webhook, error) {
	config, err := NewConfig(datadogConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create auto instrumentation config: %v", err)
	}

	// Create dynamic client using the same config pattern as the apiserver package
	clientConfig, err := apiserver.GetClientConfig(30*time.Second, 10.0, 20) // Use similar timeouts/limits as other informer clients
	if err != nil {
		return nil, fmt.Errorf("failed to get client config for dynamic client: %v", err)
	}

	dynamicClient, err := dynamic.NewForConfig(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %v", err)
	}

	store := servicemonitor.NewStore()

	dynamicInformerFactory := dynamic_informer.NewDynamicSharedInformerFactory(dynamicClient, 10*time.Second)
	watcher, err := servicemonitor.NewServiceMonitorWatcher(store, dynamicInformerFactory)
	if err != nil {
		return nil, fmt.Errorf("failed to create service monitor watcher: %v", err)
	}
	watcher.Run(make(chan struct{}))

	serviceMonitorUpdates := make(chan []servicemonitor.DatadogServiceMonitor)
	store.AddListener(serviceMonitorUpdates)

	imageResolver := NewImageResolver(rcClient, datadogConfig)

	apm, err := NewTargetMutator(config, wmeta, imageResolver, serviceMonitorUpdates)
	if err != nil {
		return nil, fmt.Errorf("failed to create auto instrumentation namespace mutator: %v", err)
	}

	// For auto instrumentation, we need all the mutators to be applied for SSI to function. Specifically, we need
	// things like the Datadog socket to be mounted from the config webhook and the DD_ENV, DD_SERVICE, and DD_VERSION
	// env vars to be set from labels if they are available..
	mutator := mutatecommon.NewMutators(
		tagsfromlabels.NewMutator(tagsfromlabels.NewMutatorConfig(datadogConfig), apm),
		configWebhook.NewMutator(configWebhook.NewMutatorConfig(datadogConfig), apm),
		apm,
	)
	labelSelectors := NewLabelSelectors(NewLabelSelectorsConfig(datadogConfig))
	return NewWebhook(config.Webhook, wmeta, mutator, labelSelectors)

}
