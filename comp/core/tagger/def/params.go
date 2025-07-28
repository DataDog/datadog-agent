// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package tagger

import (
	"crypto/tls"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// RemoteParams provides remote tagger parameters
type RemoteParams struct {
	// RemoteFilter is the filter to apply to the remote tagger when streaming tag events
	RemoteFilter *types.Filter
	// RemoteTarget function return the target in which the remote tagger will connect
	// If it returns an error we stop the application
	RemoteTarget func(config.Component) (string, error)

	// OverrideTLSConfig allows to override the TLS configuration used by the remote tagger
	// This should be used only for Cluster Agent x CLC communication
	OverrideTLSConfig *tls.Config

	// OverrideAuthTokenGetter allows to override the auth token used by the remote tagger
	// This should be used only for Cluster Agent x CLC communication
	OverrideAuthTokenGetter func(pkgconfigmodel.Reader) (string, error)
}

// DualParams provides dual tagger parameters
type DualParams struct {
	// UseRemote is a function to determine if the remote tagger should be used
	UseRemote func(config.Component) bool
}
