// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proto

import (
	remoteagent "github.com/DataDog/datadog-agent/comp/remoteagent/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func ProtobufToFlareData(in *pb.FlareData) *remoteagent.FlareData {
	out := &remoteagent.FlareData{
		AgentId: in.AgentId,
		Files:   in.Files,
	}
	return out
}

func ProtobufToStatusData(in *pb.StatusData) *remoteagent.StatusData {
	out := &remoteagent.StatusData{
		AgentId:       in.AgentId,
		MainSection:   protobufToStatusSection(in.MainSection),
		NamedSections: protobufToNamedSections(in.NamedSections),
	}
	return out
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
