// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// PeekSender returns a Sender with passed ID or an error if the sender is not registered
func (s *senders) PeekSender(cid check.ID) (Sender, error) {
	return s.senderPool.getSender(cid)
}
