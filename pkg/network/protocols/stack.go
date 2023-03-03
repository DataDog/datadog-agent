// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package protocols

type Stack struct {
	Api         ProtocolType
	Application ProtocolType
	Encryption  ProtocolType
}

func NewStack(api, application, encryption uint8) Stack {
	return Stack{
		Api:         uint16(api) | layerAPIBit,
		Application: uint16(application) | layerApplicationBit,
		Encryption:  uint16(encryption) | layerEncryptionBit,
	}
}

func (s *Stack) MergeWith(other Stack) {
	if s.Api == Unknown {
		s.Api = other.Api
	}

	if s.Application == Unknown {
		s.Application = other.Application
	}

	if s.Encryption == Unknown {
		s.Encryption = other.Encryption
	}
}
