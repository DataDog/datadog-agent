// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package authtoken implements the creation and access to the auth_token used to communicate between Agent processes.
// This component offers two implementations: one to create and fetch the auth_token and another that doesn't create the
// auth_token file but can fetch it it's available.
package authtoken

import "crypto/tls"

// team: agent-shared-components

// Component is the component type.
type Component interface {
	Get() string
	GetTLSClientConfig() *tls.Config
	GetTLSServerConfig() *tls.Config
}
