// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package proto provides functions to convert between protobuf and remoteagent types.
package proto

import (
	remoteagent "github.com/DataDog/datadog-agent/comp/remoteagent/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// ProtobufToRemoteAgentRegistration converts the protobuf representation of a remote agent registration to the internal type.
func ProtobufToRemoteAgentRegistration(in *pb.RegisterRemoteAgentRequest) *remoteagent.RegistrationData {
	return &remoteagent.RegistrationData{
		ID:          in.Id,
		DisplayName: in.DisplayName,
		APIEndpoint: in.ApiEndpoint,
		AuthToken:   in.AuthToken,
	}
}

// ProtobufToFlareData converts the protobuf representation of flare data to the internal type.
func ProtobufToFlareData(agentID string, resp *pb.GetFlareFilesResponse) *remoteagent.FlareData {
	return &remoteagent.FlareData{
		AgentID: agentID,
		Files:   resp.Files,
	}
}

// ProtobufToStatusData converts the protobuf representation of status data to the internal type.
func ProtobufToStatusData(agentID string, displayName string, resp *pb.GetStatusDetailsResponse) *remoteagent.StatusData {
	return &remoteagent.StatusData{
		AgentID:       agentID,
		DisplayName:   displayName,
		MainSection:   protobufToStatusSection(resp.MainSection),
		NamedSections: protobufToNamedSections(resp.NamedSections),
	}
}

func protobufToStatusSection(statusSection *pb.StatusSection) remoteagent.StatusSection {
	return statusSection.Fields
}

func protobufToNamedSections(namedSections map[string]*pb.StatusSection) map[string]remoteagent.StatusSection {
	sections := make(map[string]remoteagent.StatusSection, len(namedSections))

	for name, section := range namedSections {
		sections[name] = protobufToStatusSection(section)
	}

	return sections
}
