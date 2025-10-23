// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
)

// Cleanup removes all AppSec injections from the cluster resources.
// It is called when AppSec is disabled to ensure no residual configuration remains.
// Needs to be called as the leader instance to avoid conflicts.
func Cleanup(ctx context.Context, logger log.Component, datadogConfig config.Component) error {
	injector = newSecurityInjector(ctx, logger, datadogConfig, nil)
	if injector == nil {
		return nil
	}

	// Get API client for proxy detection and event recording
	apiClient, err := apiserver.GetAPIClient()
	if err != nil {
		logger.Errorf("Failed to get API client: %v", err)
		return nil
	}

	patterns := injector.CompilePatterns()
	var errs []error
	for _, pattern := range patterns {
		errs = append(errs, cleanupPattern(ctx, logger, apiClient.DynamicCl, pattern))
	}

	return errors.Join(errs...)
}

func cleanupPattern(ctx context.Context, logger log.Component, client dynamic.Interface, pattern appsecconfig.InjectionPattern) error {
	informerFactory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(client, 0, pattern.Namespace(), func(opts *metav1.ListOptions) {
		opts.LabelSelector = selector.String()
	})

	objs, err := informerFactory.ForResource(pattern.Resource()).Lister().List(labels.Everything())
	if err != nil {
		logger.Warnf("Failed to list resources for pattern %v to cleanup after disabling appsec: %v", pattern, err)
		return err
	}

	var errs []error
	for _, obj := range objs {
		unstructuredObj, ok := obj.(*unstructured.Unstructured)
		if !ok {
			logger.Warnf("Failed to cast object to unstructured from type %T", obj)
			continue
		}
		errs = append(errs, pattern.Deleted(ctx, unstructuredObj.GetNamespace(), unstructuredObj.GetName()))
	}

	return errors.Join(errs...)
}
