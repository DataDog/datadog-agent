// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeactionsimpl implements the kubeactions component interface.
package kubeactionsimpl

import (
	"context"
	"fmt"

	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

// Requires defines the dependencies for the kubeactions component.
type Requires struct {
	Lifecycle compdef.Lifecycle

	Log           log.Component
	Config        config.Component
	Hostname      hostnameinterface.Component
	APIClient     *apiserver.APIClient
	EventPlatform eventplatform.Component
	Params        kubeactions.Params
}

// Provides defines the output of the kubeactions component.
type Provides struct {
	Comp kubeactions.Component
}

type kubeactionsImpl struct {
	log         log.Component
	clusterID   string
	clusterName string
	reporter    *resultReporter
}

var _ kubeactions.Component = (*kubeactionsImpl)(nil)

// NewComponent creates a new kubeactions component.
func NewComponent(reqs Requires) (Provides, error) {
	ctx := context.Background()

	coreCl, ok := reqs.APIClient.Cl.CoreV1().(*corev1.CoreV1Client)
	if !ok {
		return Provides{}, fmt.Errorf("kubeactions: unexpected CoreV1 client type %T", reqs.APIClient.Cl.CoreV1())
	}
	clusterID, err := common.GetOrCreateClusterID(coreCl)
	if err != nil {
		return Provides{}, fmt.Errorf("kubeactions: get cluster ID: %w", err)
	}

	// clustername.GetClusterName needs the hostname as a fallback source for the
	// cluster name. An empty hostname is acceptable — detection just skips that
	// source.
	hostname, err := reqs.Hostname.Get(ctx)
	if err != nil {
		reqs.Log.Warnf("kubeactions: hostname lookup failed, continuing with empty hostname: %v", err)
		hostname = ""
	}
	clusterName := clustername.GetClusterName(ctx, hostname)

	// The Event Platform forwarder may be unavailable; the reporter is nil-safe.
	var forwarder eventplatform.Forwarder
	if fwd, ok := reqs.EventPlatform.Get(); ok {
		forwarder = fwd
	} else {
		reqs.Log.Warnf("kubeactions: Event Platform forwarder not available, result reporting will be disabled")
	}

	comp := &kubeactionsImpl{
		log:         reqs.Log,
		clusterID:   clusterID,
		clusterName: clusterName,
		reporter:    newResultReporter(forwarder, clusterName, clusterID),
	}

	reqs.Lifecycle.Append(compdef.Hook{OnStart: comp.start, OnStop: comp.stop})

	return Provides{Comp: comp}, nil
}

func (k *kubeactionsImpl) start(context.Context) error {
	k.log.Infof("Starting kubeactions component (clusterName=%s clusterID=%s)", k.clusterName, k.clusterID)
	return nil
}

func (k *kubeactionsImpl) stop(context.Context) error {
	k.log.Info("Stopping kubeactions component")
	return nil
}

// ReportReceived implements kubeactions.Component.
func (k *kubeactionsImpl) ReportReceived(report kubeactions.ActionReport) {
	k.reporter.ReportReceived(report)
}

// ReportProgress implements kubeactions.Component.
func (k *kubeactionsImpl) ReportProgress(report kubeactions.ActionReport, message string) {
	k.reporter.ReportProgress(report, message)
}

// ReportResult implements kubeactions.Component.
func (k *kubeactionsImpl) ReportResult(report kubeactions.ActionReport, result kubeactions.ExecutionResult) {
	k.reporter.ReportResult(report, result)
}
