// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeactions builds a 'kubeactions' command to manually execute Kubernetes actions.
package kubeactions

import (
	"context"
	"fmt"
	"time"

	kubeactions "github.com/DataDog/agent-payload/v5/kubeactions"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	kubeactionsreg "github.com/DataDog/datadog-agent/pkg/clusteragent/kubeactions"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	apiserver "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// GlobalParams contains the values of agent-global Cobra flags.
type GlobalParams struct {
	ConfFilePath string
	ConfigName   string
	LoggerName   string
}

type cliParams struct {
	action        string
	namespace     string
	name          string
	kind          string
	resourceID    string
	gracePeriod   int64
	patch         string
	patchStrategy string
}

// MakeCommand returns a `kubeactions` command to be used by cluster-agent binaries.
func MakeCommand(globalParamsGetter func() GlobalParams) *cobra.Command {
	cliParams := &cliParams{}

	cmd := &cobra.Command{
		Use:          "kubeactions",
		Short:        "Execute a Kubernetes action directly",
		Long:         "Manually execute a Kubernetes action (delete_pod, restart_deployment, patch_deployment) against a cluster resource.",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			globalParams := globalParamsGetter()
			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithConfigName(globalParams.ConfigName)),
					LogParams:    log.ForOneShot(globalParams.LoggerName, "off", true),
				}),
				core.Bundle(),
			)
		},
	}

	cmd.Flags().StringVar(&cliParams.action, "action", "", "Action type: delete_pod, restart_deployment, patch_deployment (required)")
	cmd.Flags().StringVar(&cliParams.namespace, "namespace", "", "Kubernetes namespace (required)")
	cmd.Flags().StringVar(&cliParams.name, "name", "", "Resource name (required)")
	cmd.Flags().StringVar(&cliParams.kind, "kind", "", "Resource kind: Pod, Deployment (required)")
	cmd.Flags().StringVar(&cliParams.resourceID, "resource-id", "", "Resource UID (required)")
	cmd.Flags().Int64Var(&cliParams.gracePeriod, "grace-period", -1, "Grace period seconds for delete_pod (-1 means default)")
	cmd.Flags().StringVar(&cliParams.patch, "patch", "", "Patch JSON string for patch_deployment")
	cmd.Flags().StringVar(&cliParams.patchStrategy, "patch-strategy", "strategic", "Patch strategy for patch_deployment: strategic, merge, json")

	_ = cmd.MarkFlagRequired("action")
	_ = cmd.MarkFlagRequired("namespace")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("kind")
	_ = cmd.MarkFlagRequired("resource-id")

	return cmd
}

func run(_ log.Component, _ config.Component, cliParams *cliParams) error {
	// Build Kubernetes clientset
	clientset, err := apiserver.GetKubeClient(10*time.Second, 10, 20)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Create executor registry and register executors
	registry := kubeactionsreg.NewExecutorRegistry(clientset)
	kubeactionsreg.RegisterExecutors(registry, clientset)

	// Build the KubeAction protobuf
	action, err := buildAction(cliParams)
	if err != nil {
		return fmt.Errorf("failed to build action: %w", err)
	}

	// Execute
	ctx := context.Background()
	result := registry.Execute(ctx, action)

	fmt.Printf("Status:  %s\nMessage: %s\n", result.Status, result.Message)

	if result.Status == kubeactionsreg.StatusFailed {
		return fmt.Errorf("action failed: %s", result.Message)
	}
	return nil
}

func buildAction(params *cliParams) (*kubeactions.KubeAction, error) {
	resource := &kubeactions.KubeResource{
		Kind:       params.kind,
		Namespace:  params.namespace,
		Name:       params.name,
		ResourceId: params.resourceID,
	}

	action := &kubeactions.KubeAction{
		Resource: resource,
	}

	switch params.action {
	case "delete_pod":
		dp := &kubeactions.DeletePodParams{}
		if params.gracePeriod >= 0 {
			dp.GracePeriodSeconds = &params.gracePeriod
		}
		action.Action = &kubeactions.KubeAction_DeletePod{
			DeletePod: dp,
		}
	case "restart_deployment":
		action.Action = &kubeactions.KubeAction_RestartDeployment{
			RestartDeployment: &kubeactions.RestartDeploymentParams{},
		}
	case "patch_deployment":
		if params.patch == "" {
			return nil, fmt.Errorf("--patch is required for patch_deployment action")
		}
		patchValue := &structpb.Value{}
		if err := patchValue.UnmarshalJSON([]byte(params.patch)); err != nil {
			return nil, fmt.Errorf("invalid patch JSON: %w", err)
		}
		action.Action = &kubeactions.KubeAction_PatchDeployment{
			PatchDeployment: &kubeactions.PatchDeploymentParams{
				Patch:         patchValue,
				PatchStrategy: params.patchStrategy,
			},
		}
	default:
		return nil, fmt.Errorf("unknown action type %q, must be one of: delete_pod, restart_deployment, patch_deployment", params.action)
	}

	return action, nil
}
