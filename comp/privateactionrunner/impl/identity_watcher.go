// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package privateactionrunnerimpl

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// startIdentityWatcher signals p.restartCh whenever the identity secret's URN changes.
func (p *PrivateActionRunner) startIdentityWatcher(ctx context.Context) {
	if !p.coreConfig.GetBool("private_action_runner.identity_use_k8s_secret") {
		return
	}

	apiClient, err := apiserver.GetAPIClient()
	if err != nil {
		p.logger.Warnf("PAR identity watcher: failed to get API client, hot-reload disabled: %v", err)
		return
	}
	if apiClient.PARIdentitySecretInformerFactory == nil {
		p.logger.Warn("PAR identity watcher: informer factory not initialized, hot-reload disabled")
		return
	}

	secretInformer := apiClient.PARIdentitySecretInformerFactory.Core().V1().Secrets().Informer()
	_, err = secretInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: p.handleIdentitySecretUpdate,
	})
	if err != nil {
		p.logger.Errorf("PAR identity watcher: failed to register event handler: %v", err)
		return
	}

	apiClient.PARIdentitySecretInformerFactory.Start(ctx.Done())
}

func (p *PrivateActionRunner) handleIdentitySecretUpdate(oldObj, newObj interface{}) {
	oldSecret, ok1 := oldObj.(*corev1.Secret)
	newSecret, ok2 := newObj.(*corev1.Secret)
	if !ok1 || !ok2 {
		return
	}
	if string(oldSecret.Data["urn"]) != string(newSecret.Data["urn"]) {
		p.logger.Info("PAR identity secret rotated, triggering credential reload")
		select {
		case p.restartCh <- struct{}{}:
		default: // a reload is already queued
		}
	}
}
