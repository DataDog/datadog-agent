// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module holds module related files
package module

import (
	"fmt"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/reporter"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// MsgSender defines a message sender
type MsgSender interface {
	Send(msg *api.SecurityEventMessage, expireFnc func(*api.SecurityEventMessage))
}

// ChanMsgSender defines a chan message sender
type ChanMsgSender struct {
	msgs chan *api.SecurityEventMessage
}

// Send the message
func (cs *ChanMsgSender) Send(msg *api.SecurityEventMessage, expireFnc func(*api.SecurityEventMessage)) {
	select {
	case cs.msgs <- msg:
		break
	default:
		// The channel is full, consume the oldest event
		select {
		case oldestMsg := <-cs.msgs:
			expireFnc(oldestMsg)
		default:
			break
		}

		// Try to send the event again
		select {
		case cs.msgs <- msg:
			break
		default:
			// Looks like the channel is full again, expire the current message too
			expireFnc(msg)
			break
		}
		break
	}
}

// NewChanMsgSender returns a new chan sender
func NewChanMsgSender(msgs chan *api.SecurityEventMessage) *ChanMsgSender {
	return &ChanMsgSender{
		msgs: msgs,
	}
}

// DirectMsgSender defines a direct sender
type DirectMsgSender struct {
	reporter common.RawReporter
}

// Send the message
func (ds *DirectMsgSender) Send(msg *api.SecurityEventMessage, _ func(*api.SecurityEventMessage)) {
	ds.reporter.ReportRaw(msg.Data, msg.Service, msg.Tags...)
}

// NewDirectMsgSender returns a new direct sender
func NewDirectMsgSender(stopper startstop.Stopper) (*DirectMsgSender, error) {
	useSecRuntimeTrack := pkgconfig.SystemProbe.GetBool("runtime_security_config.use_secruntime_track")

	endpoints, destinationsCtx, err := common.NewLogContextRuntime(useSecRuntimeTrack)
	if err != nil {
		return nil, fmt.Errorf("failed to create direct reported endpoints: %w", err)
	}

	for _, status := range endpoints.GetStatus() {
		log.Info(status)
	}

	// we set the hostname to the empty string to take advantage of the out of the box message hostname
	// resolution
	reporter, err := reporter.NewCWSReporter("", stopper, endpoints, destinationsCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create direct reporter: %w", err)
	}

	return &DirectMsgSender{
		reporter: reporter,
	}, nil
}
