// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"time"

	syslogparser "github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/syslog"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// buildStructuredMessage parses a raw syslog frame and returns a
// *message.Message in StateStructured, ready for the pipeline.
//
// The origin should be pre-configured by the tailer with source/service/tags.
// The syslog appname is used to set origin source/service if the appname is
// present and not NILVALUE ("-").
func buildStructuredMessage(frame []byte, origin *message.Origin) (*message.Message, error) {
	parsed, err := syslogparser.Parse(frame)

	sc := &message.BasicStructuredContent{
		Data: map[string]interface{}{
			"message": string(parsed.Msg),
			"syslog":  syslogparser.BuildSyslogFields(parsed),
		},
	}
	status := syslogparser.SeverityToStatus(parsed.Pri)

	msg := message.NewStructuredMessage(
		sc,
		origin,
		status,
		time.Now().UnixNano(),
	)

	// Set origin source/service from syslog appname, matching the journald pattern
	// where application name overrides source/service on the origin.
	if parsed.AppName != "" && parsed.AppName != "-" {
		origin.SetSource(parsed.AppName)
		origin.SetService(parsed.AppName)
	}

	return msg, err
}
