// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"fmt"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// RawEncoder is a shared raw encoder.
var RawEncoder Encoder = &rawEncoder{}

type rawEncoder struct{}

func (r *rawEncoder) Encode(msg *message.Message) error {
	rendered, err := msg.Render()
	if err != nil {
		return fmt.Errorf("can't render the message: %v", err)
	}

	// if the first char is '<', we can assume it's already formatted as RFC5424, thus skip this step
	// (for instance, using tcp forwarding. We don't want to override the hostname & co)
	if len(rendered) > 0 && !isRFC5424Formatted(rendered) {
		// fit RFC5424
		// <%pri%>%protocol-version% %timestamp:::date-rfc3339% %HOSTNAME% %$!new-appname% - - - %msg%\n
		extraContent := []byte("")

		// Severity
		extraContent = append(extraContent, message.StatusToSeverity(msg.GetStatus())...)

		// Protocol version
		extraContent = append(extraContent, '0')
		extraContent = append(extraContent, ' ')

		// Timestamp
		extraContent = time.Now().UTC().AppendFormat(extraContent, config.DateFormat)
		extraContent = append(extraContent, ' ')

		extraContent = append(extraContent, []byte(msg.GetHostname())...)
		extraContent = append(extraContent, ' ')

		// Service
		service := msg.Origin.Service()
		if service != "" {
			extraContent = append(extraContent, []byte(service)...)
		} else {
			extraContent = append(extraContent, '-')
		}

		// Extra
		extraContent = append(extraContent, []byte(" - - ")...)

		// Tags
		tagsPayload := msg.Origin.TagsPayload()
		if len(tagsPayload) > 0 {
			extraContent = append(extraContent, tagsPayload...)
		} else {
			extraContent = append(extraContent, '-')
		}
		extraContent = append(extraContent, ' ')

		extraContent = append(extraContent, rendered...)

		msg.SetEncoded(extraContent)
	}

	// in this situation, we don't want to re-encode the data, just
	// make sure its state is correct.
	msg.State = message.StateEncoded
	return nil
}

var rfc5424Pattern, _ = regexp.Compile("<[0-9]{1,3}>[0-9] ")

func isRFC5424Formatted(content []byte) bool {
	// RFC2424 formatted messages start with `<%pri%>%protocol-version% `
	// pri is 1 to 3 digits, protocol-version is one digit (won't realisticly
	// be more before we kill this custom code)
	// As a result, the start is between 5 and 7 chars.
	if len(content) < 8 { // even is start could be only 5 chars, RFC5424 must have other chars like `-`
		return false
	}
	return rfc5424Pattern.Match(content[:8])
}
