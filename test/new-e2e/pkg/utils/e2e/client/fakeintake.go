// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"testing"

	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/test-infra-definitions/common/utils"
	infraFakeintake "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

var _ stackInitializer = (*Fakeintake)(nil)

// A Fakeintake client that is connected to a fakeintake ECS task defined in test-infra-definition.
type Fakeintake struct {
	deserializer utils.RemoteServiceDeserializer[infraFakeintake.ClientData]
	*fakeintake.Client
}

// NewFakeintake creates a new instance of
func NewFakeintake(exporter *infraFakeintake.ConnectionExporter) *Fakeintake {
	return &Fakeintake{deserializer: exporter}
}

//lint:ignore U1000 Ignore unused function as this function is call using reflection
func (fi *Fakeintake) setStack(t *testing.T, stackResult auto.UpResult) error {
	clientData, err := fi.deserializer.Deserialize(stackResult)
	if err != nil {
		return err
	}
	fi.Client = fakeintake.NewClient("http://" + clientData.Host)
	return nil
}
