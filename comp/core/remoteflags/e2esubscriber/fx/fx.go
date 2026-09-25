// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the test-only Remote Flags subscriber.
package fx

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/remoteflags/e2esubscriber"
	"github.com/DataDog/datadog-agent/comp/core/remoteflags/types"
	"github.com/DataDog/datadog-agent/pkg/remoteflags"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/fx"
)

// Module registers the test-only Remote Flags subscriber into the
// remoteFlagSubscriber fx group. The subscriber only exposes handlers when
// `remote_flags.test_subscriber.enabled` is true; otherwise it is inert.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newSubscriber),
	)
}

func newSubscriber(cfg config.Component) types.RemoteFlagSubscriber {
	if !cfg.GetBool("remote_flags.test_subscriber.enabled") {
		return types.NewRemoteFlagSubscriber(emptySubscriber{})
	}
	log.Warn("Remote Flags test subscriber is enabled; this is intended for E2E testing only and must not be used in production")
	return types.NewRemoteFlagSubscriber(e2esubscriber.New())
}

// emptySubscriber is registered when the feature is disabled so the fx group has
// a value but contributes no handlers.
type emptySubscriber struct{}

func (emptySubscriber) Handlers() []remoteflags.FlagHandler { return nil }
