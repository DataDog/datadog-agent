// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// capturingMsgSender records the messages handed to the event sender.
type capturingMsgSender struct {
	msgs []*api.SecurityEventMessage
}

func (s *capturingMsgSender) Send(msg *api.SecurityEventMessage, _ func(*api.SecurityEventMessage)) {
	s.msgs = append(s.msgs, msg)
}

func (s *capturingMsgSender) SendTelemetry(statsd.ClientInterface) {}

type staticMarshaler struct{}

func (staticMarshaler) ToJSON() ([]byte, error) { return []byte(`{}`), nil }

// TestSendCustomEventCarriesHostname checks custom events (ruleset_loaded,
// heartbeats, self-test reports...) carry the hostname of the system-probe that
// produced them, like model events do. When they are forwarded to a remote event
// server, e.g. from a system-probe inside a micro VM, the receiving side would
// otherwise report them under its own hostname.
func TestSendCustomEventCarriesHostname(t *testing.T) {
	// No cloud provider metadata queries for the account ID tag.
	cfg := configmock.New(t)
	cfg.SetInTest("cloud_provider_metadata", []string{})

	sender := &capturingMsgSender{}
	server := &APIServer{
		probe:     &sprobe.Probe{},
		msgSender: sender,
		hostname:  "micro-vm-guest",
	}

	rule := events.NewCustomRule(events.RulesetLoadedRuleID, "New ruleset loaded", nil)
	server.SendEvent(rule, events.NewCustomEvent(model.CustomEventType, staticMarshaler{}), nil, "")

	require.Len(t, sender.msgs, 1)
	require.Equal(t, "micro-vm-guest", sender.msgs[0].Hostname)
}
