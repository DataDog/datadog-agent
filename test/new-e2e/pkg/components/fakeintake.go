// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
)

type FakeIntake struct {
	fakeintake.FakeintakeOutput

	client *client.Client
}

var _ e2e.Initializable = &FakeIntake{}

func (fi *FakeIntake) Init(e2e.Context) error {
	fi.client = client.NewClient(fi.URL)
	return nil
}

func (fi *FakeIntake) Client() *client.Client {
	return fi.client
}
