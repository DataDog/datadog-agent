// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"context"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/kubeactions/executors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/client-go/kubernetes"
)

// Setup initializes the kubeactions subsystem with all executors registered
func Setup(ctx context.Context, clientset kubernetes.Interface, isLeader func() bool, rcClient RcClient, epForwarderComp eventplatform.Component) (*ConfigRetriever, error) {
	log.Infof("Setting up Kubernetes actions subsystem")

	// Create the executor registry
	registry := NewExecutorRegistry(clientset)

	// Register all action executors
	registerExecutors(registry, clientset)

	// Create in-memory action store with TTL-based expiration
	store := NewActionStore(ctx)

	// Get the event platform forwarder if available
	log.Infof("[KubeActions] Attempting to get Event Platform forwarder from component...")
	var epForwarder eventplatform.Forwarder
	if forwarder, ok := epForwarderComp.Get(); ok {
		epForwarder = forwarder
		log.Infof("[KubeActions] SUCCESS: Event Platform forwarder available for kubeactions result reporting (forwarder=%p)", epForwarder)
	} else {
		log.Errorf("[KubeActions] CRITICAL: Event Platform forwarder not available, result reporting will be disabled")
	}

	// Create the processor with in-memory store and event platform forwarder
	log.Infof("[KubeActions] Creating ActionProcessor with epForwarder=%p", epForwarder)
	processor := NewActionProcessor(ctx, registry, store, epForwarder)
	log.Infof("[KubeActions] ActionProcessor created successfully")

	// Create and return the config retriever
	return NewConfigRetriever(ctx, processor, isLeader, rcClient)
}

// executorAdapter adapts an executors.Executor to an ActionExecutor
type executorAdapter struct {
	exec executors.Executor
}

func (a *executorAdapter) Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult {
	result := a.exec.Execute(ctx, action)
	return ExecutionResult{
		Status:  result.Status,
		Message: result.Message,
	}
}

// registerExecutors registers all available action executors
func registerExecutors(registry *ExecutorRegistry, clientset kubernetes.Interface) {
	// Register delete_pod executor
	registry.Register("delete_pod", &executorAdapter{exec: executors.NewDeletePodExecutor(clientset)})
	log.Infof("Registered executor for action type: delete_pod")

	// Register restart_deployment executor
	registry.Register("restart_deployment", &executorAdapter{exec: executors.NewRestartDeploymentExecutor(clientset)})
	log.Infof("Registered executor for action type: restart_deployment")

	// TODO: Add more executors here as they are implemented:
	// registry.Register("patch_deployment", &executorAdapter{exec: executors.NewPatchDeploymentExecutor(clientset)})
	// registry.Register("drain_node", &executorAdapter{exec: executors.NewDrainNodeExecutor(clientset)})
	// registry.Register("scale_deployment", &executorAdapter{exec: executors.NewScaleDeploymentExecutor(clientset)})
	// registry.Register("cordon_node", &executorAdapter{exec: executors.NewCordonNodeExecutor(clientset)})
	// registry.Register("uncordon_node", &executorAdapter{exec: executors.NewUncordonNodeExecutor(clientset)})
}
