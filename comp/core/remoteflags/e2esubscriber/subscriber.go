// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package e2esubscriber provides a test-only Remote Flags subscriber used by the
// remote-flags E2E suite to exercise the full FlagHandler lifecycle end-to-end:
// OnChange, IsHealthy, the health monitor, SafeRecover and OnNoConfig.
//
// It is NOT meant for production use. Registration is gated behind the
// `remote_flags.test_subscriber.enabled` config field (default false); when the
// field is false the subscriber exposes no handlers and is completely inert.
//
// The subscriber exposes two flags that share state so an E2E test can drive the
// recover path deterministically and observe it purely through the agent
// configuration dump (no log scraping required):
//
//   - feature flag ("e2e_feature") is bound to a configuration_field. When the
//     handler is recovered, the Remote Flags client unsets that field from the
//     RC source, so the config value reverting is the observable proof that
//     SafeRecover ran.
//   - fault flag ("e2e_fault") flips a shared "unhealthy" bit. Enabling it forces
//     the feature handler's IsHealthy to return false, which the health monitor
//     turns into a SafeRecover; disabling it lets the recovery probe confirm the
//     component is healthy again.
package e2esubscriber

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/remoteflags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// FeatureFlagName is the flag whose handler participates in the health
	// monitor / recover lifecycle. It is bound to FeatureConfigField.
	FeatureFlagName = "e2e_feature"
	// FaultFlagName is the flag that toggles the shared unhealthy bit, used to
	// force the feature handler into an unhealthy state.
	FaultFlagName = "e2e_fault"
	// FeatureConfigField is the configuration field the feature flag mirrors.
	// It is an existing boolean setting (default false) chosen purely as an
	// observable: the E2E asserts on its resolved value, which flips to true on
	// apply and back to false when SafeRecover unsets the RC source.
	FeatureConfigField = "logs_enabled"

	logPrefix = "[remoteflags-e2e]"
)

// sharedState is shared between the two handlers of a single subscriber.
type sharedState struct {
	// faultInjected, when true, makes the feature handler report unhealthy.
	faultInjected atomic.Bool
}

// Subscriber is the test-only RemoteFlagSubscriber. It implements
// remoteflags.RemoteFlagSubscriber.
type Subscriber struct {
	handlers []remoteflags.FlagHandler
}

// New returns a Subscriber exposing the feature and fault handlers.
func New() *Subscriber {
	shared := &sharedState{}
	return &Subscriber{
		handlers: []remoteflags.FlagHandler{
			&featureHandler{shared: shared},
			&faultHandler{shared: shared},
		},
	}
}

// Handlers implements remoteflags.RemoteFlagSubscriber.
func (s *Subscriber) Handlers() []remoteflags.FlagHandler {
	return s.handlers
}

// featureHandler is the flag whose lifecycle the E2E observes. Its OnChange
// mirrors through FeatureConfigField (handled by the client), and its health is
// controlled by the shared fault bit.
type featureHandler struct {
	shared *sharedState
}

func (h *featureHandler) FlagName() remoteflags.FlagName { return FeatureFlagName }

func (h *featureHandler) OnChange(value remoteflags.FlagValue) error {
	log.Infof("%s feature OnChange value=%t", logPrefix, bool(value))
	return nil
}

func (h *featureHandler) OnNoConfig() {
	log.Infof("%s feature OnNoConfig", logPrefix)
}

func (h *featureHandler) SafeRecover(err error, failedValue remoteflags.FlagValue) {
	log.Infof("%s feature SafeRecover failedValue=%t err=%v", logPrefix, bool(failedValue), err)
}

func (h *featureHandler) IsHealthy() bool {
	healthy := !h.shared.faultInjected.Load()
	log.Debugf("%s feature IsHealthy=%t", logPrefix, healthy)
	return healthy
}

// faultHandler toggles the shared unhealthy bit. It never needs to be healthy
// itself; the health monitor only runs for a handler while its own flag is
// enabled, and IsHealthy=true keeps it quiet.
type faultHandler struct {
	shared *sharedState
}

func (h *faultHandler) FlagName() remoteflags.FlagName { return FaultFlagName }

func (h *faultHandler) OnChange(value remoteflags.FlagValue) error {
	h.shared.faultInjected.Store(bool(value))
	log.Infof("%s fault OnChange faultInjected=%t", logPrefix, bool(value))
	return nil
}

func (h *faultHandler) OnNoConfig() {
	h.shared.faultInjected.Store(false)
	log.Infof("%s fault OnNoConfig (cleared)", logPrefix)
}

func (h *faultHandler) SafeRecover(_ error, _ remoteflags.FlagValue) {}

func (h *faultHandler) IsHealthy() bool { return true }
