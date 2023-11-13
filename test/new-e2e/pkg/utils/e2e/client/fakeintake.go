// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/common/utils"
	infraFakeintake "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

var _ pulumiStackInitializer = (*Fakeintake)(nil)

// A Fakeintake client that is connected to a fakeintake ECS task defined in test-infra-definition.
type Fakeintake struct {
	deserializer utils.RemoteServiceDeserializer[infraFakeintake.ClientData]
	*fakeintake.Client
}

// NewFakeintake creates a new instance of a fakeintake connected to a [infraFakeintake.ConnectionExporter].
//
// [infraFakeintake.ConnectionExporter]: https://pkg.go.dev/github.com/DataDog/test-infra-definitions@main/components/datadog/fakeintake#ConnectionExporter
func NewFakeintake(exporter *infraFakeintake.ConnectionExporter) *Fakeintake {
	return &Fakeintake{deserializer: exporter}
}

// initFromPulumiStack initializes the instance from the data stored in the pulumi stack.
// This method is called by [CallStackInitializers] using reflection.
//
//lint:ignore U1000 Ignore unused function as this function is called using reflection
func (fi *Fakeintake) initFromPulumiStack(t *testing.T, stackResult auto.UpResult) error {
	clientData, err := fi.deserializer.Deserialize(stackResult)
	if err != nil {
		return err
	}
	fi.Client = fakeintake.NewClient(t, "http://"+clientData.Host)
	return nil
}
