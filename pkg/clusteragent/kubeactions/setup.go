// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeactions

import (
	"context"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/kubeactions/executors"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Setup initializes the kubeactions subsystem with all executors registered.
// restConfig is used by exec-based actions (exec_command) to open streaming
// connections to the API server; it may be nil if such actions are not needed.
func Setup(ctx context.Context, clientset kubernetes.Interface, dynamicClient dynamic.Interface, restConfig *rest.Config, clusterName, clusterID string, isLeader func() bool, rcClient RcClient, epForwarderComp eventplatform.Component) (*ConfigRetriever, error) {
	log.Infof("[KubeActions] Setting up Kubernetes actions subsystem")

	// Create the executor registry
	registry := NewExecutorRegistry(clientset)

	// Register all action executors
	registerExecutors(registry, clientset, dynamicClient, restConfig)

	// Create in-memory action store with TTL-based expiration
	store := NewActionStore(ctx)

	// Get the event platform forwarder if available
	var epForwarder eventplatform.Forwarder
	if forwarder, ok := epForwarderComp.Get(); ok {
		epForwarder = forwarder
	} else {
		log.Warnf("[KubeActions] Event Platform forwarder not available, result reporting will be disabled")
	}

	processor := NewActionProcessor(ctx, registry, store, epForwarder, clusterName, clusterID)

	return NewConfigRetriever(processor, isLeader, rcClient), nil
}

// executorAdapter adapts an executors.Executor to an ActionExecutor
type executorAdapter struct {
	exec executors.Executor
}

func (a *executorAdapter) Execute(ctx context.Context, action *kubeactions.KubeAction) ExecutionResult {
	result := a.exec.Execute(ctx, action)
	return ExecutionResult{
		Status:   result.Status,
		Message:  result.Message,
		Payloads: result.Payloads,
	}
}

// registerExecutors registers all available action executors
func registerExecutors(registry *ExecutorRegistry, clientset kubernetes.Interface, dynamicClient dynamic.Interface, restConfig *rest.Config) {
	registry.Register(ActionTypeDeletePod, &executorAdapter{exec: executors.NewDeletePodExecutor(clientset)})
	registry.Register(ActionTypeRestartDeployment, &executorAdapter{exec: executors.NewRestartDeploymentExecutor(clientset)})
	registry.Register(ActionTypePatchDeployment, &executorAdapter{exec: executors.NewPatchDeploymentExecutor(clientset)})
	registry.Register(ActionTypeRollbackDeployment, &executorAdapter{exec: executors.NewRollbackDeploymentExecutor(clientset)})
	registry.Register(ActionTypeGetResource, &executorAdapter{exec: executors.NewGetResourceExecutor(dynamicClient)})
	if restConfig != nil {
		registry.Register(ActionTypeExecCommand, &executorAdapter{exec: executors.NewExecCommandExecutor(clientset, restConfig)})
	}
}
