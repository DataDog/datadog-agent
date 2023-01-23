// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package patch

import (
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/client-go/kubernetes"
)

// ControllerContext holds necessary context for the patch controller
type ControllerContext struct {
	IsLeaderFunc        func() bool
	LeaderSubscribeFunc func() <-chan struct{}
	K8sClient           kubernetes.Interface
	RcClient            *remote.Client
	ClusterName         string
	StopCh              chan struct{}
}

// StartControllers starts the patch controllers
func StartControllers(ctx ControllerContext) error {
	log.Info("Starting patch controllers")
	provider, err := newPatchProvider(ctx.RcClient, ctx.LeaderSubscribeFunc(), ctx.ClusterName)
	if err != nil {
		return err
	}
	patcher := newPatcher(ctx.K8sClient, ctx.IsLeaderFunc, provider)
	go provider.start(ctx.StopCh)
	go patcher.start(ctx.StopCh)
	return nil
}
