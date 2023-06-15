// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"testing"

	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
	infraFakeintake "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
)

var _ clientService[infraFakeintake.ClientData] = (*Fakeintake)(nil)

// A client fakeintake that is connected to a fakeintake ECS task defined in test-infra-definition.
type Fakeintake struct {
	*UpResultDeserializer[infraFakeintake.ClientData]
	*fakeintake.Client
}

// Create a new instance of
func NewFakeintake(exporter *infraFakeintake.ConnectionExporter) *Fakeintake {
	fi := &Fakeintake{}
	fi.UpResultDeserializer = NewUpResultDeserializer[infraFakeintake.ClientData](exporter, fi)
	return fi
}

//lint:ignore U1000 Ignore unused function as this function is call using reflection
func (fi *Fakeintake) initService(t *testing.T, data *infraFakeintake.ClientData) error {
	fi.Client = fakeintake.NewClient("http://" + data.Host)
	return nil
}
