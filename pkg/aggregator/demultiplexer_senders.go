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
	agg           *BufferedAggregator // TODO(remy): do we really want to store this here?
}

func newSenders(aggregator *BufferedAggregator) *senders {
	return &senders{
		agg: aggregator,
	}
}
// getDefaultSender returns a default sender.
func (s *senders) GetDefaultSender() (Sender, error) {
	s.senderInit.Do(func() {
		var defaultCheckID check.ID // the default value is the zero value
		s.defaultSender = newCheckSender(defaultCheckID,
			s.agg.hostname,
			s.agg.checkItems,
			s.agg.serviceCheckIn,
			s.agg.eventIn,
			s.agg.orchestratorMetadataIn,
			s.agg.orchestratorManifestIn,
			s.agg.eventPlatformIn,
			s.agg.contLcycleIn,
			s.agg.tagsStore,
		)
	})
	return s.defaultSender, nil
}

// NewSender creates a new sender for a check.
func (s *senders) NewSender(id check.ID) Sender {
	sender := newCheckSender(id,
			s.agg.hostname,
			s.agg.checkItems,
			s.agg.serviceCheckIn,
			s.agg.eventIn,
			s.agg.orchestratorMetadataIn,
			s.agg.orchestratorManifestIn,
			s.agg.eventPlatformIn,
			s.agg.contLcycleIn,
			s.agg.tagsStore,
	)
	return sender
}
