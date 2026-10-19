// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	"github.com/DataDog/datadog-agent/comp/logs-library/diagnostic"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type hostnameTestTap func(*message.Message)

func (f hostnameTestTap) Tap(msg *message.Message) { f(msg) }

type hostnameTestEncoder func(*message.Message, string) error

func (f hostnameTestEncoder) Encode(msg *message.Message, hostname string) error {
	return f(msg, hostname)
}

func TestHostnameResolvedBeforeTap(t *testing.T) {
	for _, tc := range []struct {
		name             string
		messageHostname  string
		withoutComponent bool
		expected         string
	}{
		{name: "default", expected: "agent-host"},
		{name: "explicit", messageHostname: "message-host", expected: "message-host"},
		{name: "unknown fallback", withoutComponent: true, expected: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hostname, _ := hostnameinterface.NewMock("agent-host")
			if tc.withoutComponent {
				hostname = nil
			}
			output := make(chan *message.Message, 1)
			order := []string{}
			tap := hostnameTestTap(func(msg *message.Message) {
				order = append(order, "tap")
				assert.Equal(t, tc.expected, msg.GetHostname())
				assert.Equal(t, []byte("probe"), msg.GetContent())
			})
			encoder := hostnameTestEncoder(func(msg *message.Message, resolved string) error {
				order = append(order, "encoder")
				assert.Equal(t, tc.expected, resolved)
				assert.Equal(t, resolved, msg.GetHostname())
				return nil
			})
			p := New(nil, nil, output, nil, encoder, tap, &diagnostic.NoopMessageReceiver{},
				hostname, metrics.NewNoopPipelineMonitor(""), "hostname-test")
			msg := message.NewMessage([]byte("probe"), nil, "", 0)
			msg.Hostname = tc.messageHostname
			p.processMessage(msg)
			assert.Equal(t, []string{"tap", "encoder"}, order)
			assert.Same(t, msg, <-output)
		})
	}
}
