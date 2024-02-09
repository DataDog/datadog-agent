// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"errors"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// senders are the sender used and provided by the Demultiplexer for checks
// to send metrics.
type senders struct {
	senderInit    sync.Once
	defaultSender sender.Sender
	senderPool    *checkSenderPool
	agg           *BufferedAggregator // TODO(remy): do we really want to store this here?
}

func newSenders(aggregator *BufferedAggregator) *senders {
	return &senders{
		agg: aggregator,
		senderPool: &checkSenderPool{
			agg:     aggregator,
			senders: make(map[checkid.ID]sender.Sender),
		},
	}
}

// SetSender returns the passed sender with the passed ID.
// This is largely for testing purposes
func (s *senders) SetSender(sender sender.Sender, id checkid.ID) error {
	if s == nil {
		return errors.New("Demultiplexer was not initialized")
	}
	return s.senderPool.setSender(sender, id)
}

// GetSender returns a sender.Sender with passed ID, properly registered with the aggregator
// If no error is returned here, DestroySender must be called with the same ID
// once the sender is not used anymore
func (s *senders) GetSender(cid checkid.ID) (sender.Sender, error) {
	sender, err := s.senderPool.getSender(cid)
	if err != nil {
		sender, err = s.senderPool.mkSender(cid)
	}
	return sender, err
}

// DestroySender frees up the resources used by the sender with passed ID (by deregistering it from the aggregator)
// Should be called when no sender with this ID is used anymore
// The metrics of this (these) sender(s) that haven't been flushed yet will be lost
func (s *senders) DestroySender(id checkid.ID) {
	s.senderPool.removeSender(id)
}

// GetDefaultSender returns a default sender.
func (s *senders) GetDefaultSender() (sender.Sender, error) {
	s.senderInit.Do(func() {
		var defaultCheckID checkid.ID        // the default value is the zero value
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
