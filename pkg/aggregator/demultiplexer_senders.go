// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// senders are the sender used and provided by the Demultiplexer for checks
// to send metrics.
type senders struct {
	senderInit    sync.Once
	defaultSender Sender
	senderPool    *checkSenderPool
	agg           *BufferedAggregator // TODO(remy): do we really want to store this here?
}

func newSenders(aggregator *BufferedAggregator) *senders {
	return &senders{
		agg: aggregator,
		senderPool: &checkSenderPool{
			agg:     aggregator,
			senders: make(map[check.ID]Sender),
		},
	}
}

// SetSender returns the passed sender with the passed ID.
// This is largely for testing purposes
func (s *senders) SetSender(sender Sender, id check.ID) error {
	return s.senderPool.setSender(sender, id)
}

// cleanSenders cleans the senders list, used in unit tests.
func (s *senders) cleanSenders() {
	s.senderPool.senders = make(map[check.ID]Sender)
	s.senderInit = sync.Once{}
}

// GetSender returns a Sender with passed ID, properly registered with the aggregator
// If no error is returned here, DestroySender must be called with the same ID
// once the sender is not used anymore
func (s *senders) GetSender(cid check.ID) (Sender, error) {
	sender, err := s.senderPool.getSender(cid)
	if err != nil {
		sender, err = s.senderPool.mkSender(cid)
	}
	return sender, err
}

// DestroySender frees up the resources used by the sender with passed ID (by deregistering it from the aggregator)
// Should be called when no sender with this ID is used anymore
// The metrics of this (these) sender(s) that haven't been flushed yet will be lost
func (s *senders) DestroySender(id check.ID) {
	s.senderPool.removeSender(id)
}

// getDefaultSender returns a default sender.
func (s *senders) GetDefaultSender() (Sender, error) {
	s.senderInit.Do(func() {
		var defaultCheckID check.ID          // the default value is the zero value
		s.agg.registerSender(defaultCheckID) //nolint:errcheck
		s.defaultSender = newCheckSender(defaultCheckID,
			s.agg.hostname,
			s.agg.checkItems,
			s.agg.serviceCheckIn,
			s.agg.eventIn,
			s.agg.orchestratorMetadataIn,
			s.agg.orchestratorManifestIn,
			s.agg.eventPlatformIn,
		)
	})
	return s.defaultSender, nil
}
