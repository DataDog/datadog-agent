// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

type sourceID uint8

const (
	sourceIDDefault sourceID = iota
	sourceIDUnknown
	sourceIDInfraMode
	sourceIDFile
	sourceIDEnvVar
	sourceIDFleetPolicies
	sourceIDAgentRuntime
	sourceIDLocalConfigProcess
	sourceIDRC
	sourceIDCLI
)

func sourceIDFromSource(s model.Source) sourceID {
	switch s {
	case model.SourceDefault:
		return sourceIDDefault
	case model.SourceUnknown:
		return sourceIDUnknown
	case model.SourceInfraMode:
		return sourceIDInfraMode
	case model.SourceFile:
		return sourceIDFile
	case model.SourceEnvVar:
		return sourceIDEnvVar
	case model.SourceFleetPolicies:
		return sourceIDFleetPolicies
	case model.SourceAgentRuntime:
		return sourceIDAgentRuntime
	case model.SourceLocalConfigProcess:
		return sourceIDLocalConfigProcess
	case model.SourceRC:
		return sourceIDRC
	case model.SourceCLI:
		return sourceIDCLI
	default:
		return sourceIDUnknown
	}
}

func (s sourceID) toSource() model.Source {
	switch s {
	case sourceIDDefault:
		return model.SourceDefault
	case sourceIDUnknown:
		return model.SourceUnknown
	case sourceIDInfraMode:
		return model.SourceInfraMode
	case sourceIDFile:
		return model.SourceFile
	case sourceIDEnvVar:
		return model.SourceEnvVar
	case sourceIDFleetPolicies:
		return model.SourceFleetPolicies
	case sourceIDAgentRuntime:
		return model.SourceAgentRuntime
	case sourceIDLocalConfigProcess:
		return model.SourceLocalConfigProcess
	case sourceIDRC:
		return model.SourceRC
	case sourceIDCLI:
		return model.SourceCLI
	default:
		return model.SourceUnknown
	}
}

func (s sourceID) IsGreaterThan(other sourceID) bool {
	return s > other
}
