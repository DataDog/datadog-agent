// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"context"
	"errors"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"k8s.io/utils/clock"
)

// StartSpotScheduling creates and starts the spot scheduler, returning a PodHandler for use in the admission webhook.
func StartSpotScheduling(ctx context.Context, wlm workloadmeta.Component, apiCl *apiserver.APIClient, isLeaderFunc func() bool) (PodHandler, error) {
	if apiCl == nil {
		return nil, errors.New("impossible to start spot scheduling without valid APIClient")
	}

	cfg := ReadConfig(pkgconfigsetup.Datadog())
	s := newScheduler(cfg, clock.RealClock{}, wlm,
		newKubePodEvictor(apiCl.Cl),
		newKubeWorkloadPatcher(apiCl.DynamicInformerCl),
		apiCl.DynamicInformerCl,
		isLeaderFunc)
	s.Start(ctx)

	return s, nil
}
