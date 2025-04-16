// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package report implements Versa metadata and metrics reporting
package report

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// Sender implements methods for sending Versa metrics and metadata
type Sender struct {
	sender       sender.Sender
	namespace    string
	lastTimeSent map[string]float64
	deviceTags   map[string][]string
}

// NewSender returns a new VersaSender
func NewSender(sender sender.Sender, namespace string) *Sender {
	return &Sender{
		sender:       sender,
		namespace:    namespace,
		lastTimeSent: make(map[string]float64),
		deviceTags:   make(map[string][]string),
	}
}
