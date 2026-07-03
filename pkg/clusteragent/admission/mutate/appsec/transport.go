// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import appsecconfig "github.com/DataDog/datadog-agent/pkg/clusteragent/appsec/config"

// Transport is the sidecar mutation transport metric tag value.
type Transport string

const (
	TransportUDS         Transport = "uds"
	TransportTCP         Transport = "tcp"
	TransportNginxModule Transport = "nginx_module"
)

// transportByProxyType is valid only for SIDECAR-mode emission. It is consulted solely by the
// sidecar admission path (callPattern), the only place the transport tag is produced. A future
// proxy that supports multiple transports, such as external mode, must not rely on this
// proxy_type-to-transport table.
var transportByProxyType = map[appsecconfig.ProxyType]Transport{
	appsecconfig.ProxyTypeEnvoyGateway: TransportUDS,
	appsecconfig.ProxyTypeIstio:        TransportTCP,
	appsecconfig.ProxyTypeIstioGateway: TransportTCP,
	appsecconfig.ProxyTypeIngressNginx: TransportNginxModule,
}
