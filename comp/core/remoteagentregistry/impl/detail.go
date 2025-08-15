// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type remoteAgentDetails struct {
	lastSeen             time.Time
	displayName          string
	sanatizedDisplayName string
	apiEndpoint          string
	client               pb.RemoteAgentClient
}

func newRemoteAgentDetails(registration *remoteagentregistry.RegistrationData, config config.Component) (*remoteAgentDetails, error) {
	client, err := newRemoteAgentClient(registration)
	if err != nil {
		return nil, err
	}

	rad := &remoteAgentDetails{
		displayName:          registration.DisplayName,
		apiEndpoint:          registration.APIEndpoint,
		sanatizedDisplayName: sanatizeString(registration.DisplayName),
		client:               client,
		lastSeen:             time.Now(),
	}

	return rad, nil
}
