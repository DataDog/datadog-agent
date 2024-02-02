// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

// Package senderhelper provides a set of fx options for providing a mock
// sender for the demultiplexer.
package senderhelper

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// Opts is a set of options for providing a demux with a mock sender.
// We can remove this if the Sender is ever exposed as a component.
var Opts = fx.Options(
	defaultforwarder.MockModule(),
	demultiplexerimpl.MockModule(),
	config.MockModule(),
	fx.Provide(func() (*mocksender.MockSender, sender.Sender) {
		mockSender := mocksender.NewMockSender("mock-sender")
		mockSender.SetupAcceptAll()
		return mockSender, mockSender
	}),
	fx.Decorate(func(demux demultiplexer.Mock, s sender.Sender) demultiplexer.Component {
		demux.SetDefaultSender(s)
		return demux
	}),
)
