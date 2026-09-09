// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	"context"
	stdErrors "errors"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// Cleanup removes all AppSec injections from the cluster resources.
// It is called when AppSec is disabled to ensure no residual configuration remains.
// Needs to be called as the leader instance to avoid conflicts.
func Cleanup(ctx context.Context, logger log.Component, datadogConfig config.Component, leaderSub leaderNotifier) {
	logger.Info("Cleaning up appsec injections from cluster resources because proxy injection is disabled")
	cleanupInjector := newSecurityInjector(ctx, logger, appsecconfig.FromComponent(datadogConfig, logger), leaderSub)
	if cleanupInjector == nil {
		return
	}

	apiClient, err := apiserver.GetAPIClient()
	if err != nil {
		logger.Warnf("Failed to get API client: %v", err)
		return
	}

	leaderNotifChange, isLeader := leaderSub()

	go func() {
		// Wait to become leader
		for !isLeader() {
			<-leaderNotifChange
		}

		for _, pattern := range cleanupInjector.patterns {
			cleanupPattern(ctx, logger, apiClient.DynamicCl, pattern)
		}
	}()
}

func cleanupPattern(ctx context.Context, logger log.Component, client dynamic.Interface, pattern appsecconfig.InjectionPattern) {
	objs, err := client.Resource(pattern.Resource()).Namespace(pattern.Namespace()).List(ctx, metav1.ListOptions{})
	if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
		// The resource type (CRD) is not installed in this cluster, so there is
		// nothing of this proxy type to clean up. Cleanup iterates over every
		// registered proxy pattern, so this is expected on clusters that only
		// run a subset of the supported proxies.
		logger.Debugf("Skipping cleanup for pattern %v: resource type %v is not present in the cluster: %v", pattern, pattern.Resource(), err)
		return
	}

	if apierrors.IsForbidden(err) {
		logger.Debugf("Skipping cleanup of resource pattern %v due to forbidden access: %v", pattern, err)
		return
	}

	if err != nil {
		logger.Warnf("Failed to list resources for pattern %v to cleanup after disabling appsec: %v", pattern, err)
		return
	}

	if len(objs.Items) != 0 {
		logger.Debugf("Cleaning up appsec injections %d objects from resource pattern: %v", len(objs.Items), pattern.Resource())
	}

	var errs []error
	for _, obj := range objs.Items {
		errs = append(errs, pattern.Deleted(ctx, &obj))
	}

	if errs := stdErrors.Join(errs...); errs != nil {
		logger.Debugf("Errors occurred during cleanup of resource pattern %v: %v", pattern.Resource(), errs)
	}
}
