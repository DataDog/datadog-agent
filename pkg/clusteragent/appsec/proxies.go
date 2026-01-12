// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	"context"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/envoygateway"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/istio"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
)

// ProxyConstructor is the type of function that creates a new InjectionPattern for a given proxy
type ProxyConstructor func(
	k8sClient dynamic.Interface,
	logger log.Component,
	config appsecconfig.Config,
	eventRecorder record.EventRecorder) appsecconfig.InjectionPattern

var proxyConstructorMap = map[appsecconfig.ProxyType]ProxyConstructor{
	appsecconfig.ProxyTypeEnvoyGateway: envoygateway.New,
	appsecconfig.ProxyTypeIstio:        istio.New,
}

// ProxyDetector is the type of function that detects if a given proxy is installed in the cluster
// it is called before using the [ProxyConstructor] to avoid creating a controller for a proxy that is not present
// during the auto-detection phase at startup.
type ProxyDetector func(
	ctx context.Context,
	k8sClient dynamic.Interface) (bool, error)

var proxyDetectionMap = map[appsecconfig.ProxyType]ProxyDetector{
	appsecconfig.ProxyTypeEnvoyGateway: envoygateway.Detect,
	appsecconfig.ProxyTypeIstio:        istio.Detect,
}
