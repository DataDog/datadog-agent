// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"context"
	"fmt"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/kubeactions/executors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/client-go/kubernetes"
)

// Setup initializes the kubeactions subsystem with all executors registered
// If namespace is empty, uses "default" namespace for persistent storage
func Setup(ctx context.Context, clientset kubernetes.Interface, namespace string, isLeader func() bool, rcClient RcClient) (*ConfigRetriever, error) {
	log.Infof("Setting up Kubernetes actions subsystem")

	// Create the executor registry
	registry := NewExecutorRegistry(clientset)

	// Register all action executors
	registerExecutors(registry, clientset)

	// Create persistent action store
	if namespace == "" {
		namespace = ConfigMapNamespace
	}
	store, err := NewPersistentActionStore(ctx, clientset, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create persistent action store: %w", err)
	}

	// Create the processor with persistent store
	processor := NewActionProcessor(ctx, registry, store)

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
