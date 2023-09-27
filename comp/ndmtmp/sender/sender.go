// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package sender exposes a Sender for netflow.
package sender

import (
	"github.com/DataDog/datadog-agent/comp/ndmtmp/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func getDefaultSender(agg aggregator.Component) (Component, error) {
	return agg.GetDefaultSender()
}

func newMockSender() MockComponent {
	mockSender := mocksender.NewMockSender("mock-sender")
	mockSender.SetupAcceptAll()
	return mockSender
}
