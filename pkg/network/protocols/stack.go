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
	stack := Stack{}

	if api > 0 {
		stack.Api = uint16(api) | layerAPIBit
	}

	if application > 0 {
		stack.Application = uint16(application) | layerApplicationBit
	}

	if encryption > 0 {
		stack.Encryption = uint16(encryption) | layerEncryptionBit
	}

	return stack
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

func (s *Stack) Contains(proto ProtocolType) bool {
	return s.Api == proto || s.Application == proto || s.Encryption == proto
}
