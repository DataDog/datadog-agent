// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oteltest

const (
	semconv161ContainerIDKey           = "container.id"
	semconv161K8SContainerNameKey      = "k8s.container.name"
	semconv161ServiceNameKey           = "service.name"
	semconv161DeploymentEnvironmentKey = "deployment.environment"
	semconv161HTTPStatusCodeKey        = "http.status_code"
	semconv161PeerServiceKey           = "peer.service"
	semconv161DBSystemKey              = "db.system"
	semconv161DBStatementKey           = "db.statement"
	semconv161HTTPMethodKey            = "http.method"
	semconv161HTTPRouteKey             = "http.route"
	semconv161RPCMethodKey             = "rpc.method"
	semconv161RPCServiceKey            = "rpc.service"
)
