// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

// Payload is an interface shared by the output of the newer metadata providers.
// Right now this interface simply satisfies the Protobuf interface.
type Payload interface {
	Reset()
	String() string
	ProtoMessage()
	Descriptor() ([]byte, []int)
}
