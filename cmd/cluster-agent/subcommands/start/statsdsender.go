// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package start

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/security/telemetry"
)

func simpleTelemetrySenderFromSenderManager(senderManager sender.SenderManager) (telemetry.SimpleTelemetrySender, error) {
	s, err := senderManager.GetDefaultSender()
	if err != nil {
		return nil, err
	}
	return &statsdFromSender{sender: s}, nil
}

type statsdFromSender struct {
	sender sender.Sender
}

// Gauge implements telemetry.SimpleTelemetrySender.
func (s *statsdFromSender) Gauge(name string, value float64, tags []string) {
	s.sender.Gauge(name, value, "", tags)
}

// Commit implements telemetry.SimpleTelemetrySender.
func (s *statsdFromSender) Commit() {
	s.sender.Commit()
}
