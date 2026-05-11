// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package networkconfigmanagementimpl implements a stub component when ncm is disabled.
package networkconfigmanagementimpl

import (
	"net/http"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Provides defines the output of the networkconfigmanagement component
type Provides struct {
	compdef.Out

	Comp              option.Option[networkconfigmanagement.Component]
	GetConfigEndpoint api.EndpointProvider `group:"agent_endpoint"`
}

func nilEndpoint() api.EndpointProvider {
	return api.NewAgentEndpointProvider(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error": "ncm not enabled for agent"}`, http.StatusBadRequest)
	}, "/ncm/config", "GET").Provider
}
