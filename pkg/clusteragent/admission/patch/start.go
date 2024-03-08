// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/controllers/webhook"
	autoinstrumentation "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/telemetry"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/client-go/kubernetes"
)

// ControllerContext holds necessary context for the patch controller
type ControllerContext struct {
	IsLeaderFunc        func() bool
	LeaderSubscribeFunc func() <-chan struct{}
	K8sClient           kubernetes.Interface
	RcClient            *rcclient.Client
	ClusterName         string
	ClusterID           string
	StopCh              chan struct{}
}

// StartControllers starts the patch controllers
func StartControllers(ctx ControllerContext) ([]webhook.MutatingWebhook, error) {
	log.Info("Starting patch controllers")
	telemetryCollector := telemetry.NewNoopCollector()
	if ctx.RcClient != nil {
		telemetryCollector = telemetry.NewCollector(ctx.RcClient.ID, ctx.ClusterID)
	}

	var webhooks []webhook.MutatingWebhook
	webhook, err := autoinstrumentation.NewWebhook()
	if err == nil {
		webhooks = append(webhooks, webhook)
	} else {
		log.Errorf("failed to register APM Instrumentation webhook: %v", err)
	}

	provider, err := newAPMProvider(ctx.RcClient, ctx.LeaderSubscribeFunc(), telemetryCollector, ctx.ClusterName, webhook)

	if err != nil {
		return webhooks, err
	}
	go provider.start(ctx.StopCh)
	return webhooks, nil
}
