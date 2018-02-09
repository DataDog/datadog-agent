// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package processor

import (
	"bytes"
	"time"

	"regexp"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pb"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// Encoder turns a message into a raw byte array ready to be sent.
type Encoder interface {
	encode(msg message.Message, redactedMsg []byte) ([]byte, error)
}

// Raw is an encoder implementation that writes messages as raw strings.
var rawEncoder raw

// Proto is an encoder implementation that writes messages as protocol buffers.
var protoEncoder proto

// NewEncoder returns an encoder.
func NewEncoder(useProto bool) Encoder {
	if useProto {
		return &protoEncoder
	}
	return &rawEncoder
}

var rfc5424Pattern, _ = regexp.Compile("<[0-9]{1,3}>[0-9] ")

type raw struct{}

func (r *raw) encode(msg message.Message, redactedMsg []byte) ([]byte, error) {

	// if the first char is '<', we can assume it's already formatted as RFC5424, thus skip this step
	// (for instance, using tcp forwarding. We don't want to override the hostname & co)
	if len(msg.Content()) > 0 && !r.isRFC5424Formatted(msg.Content()) {
		// fit RFC5424
		// <%pri%>%protocol-version% %timestamp:::date-rfc3339% %HOSTNAME% %$!new-appname% - - - %msg%\n
		extraContent := []byte("")

		// Severity
		if msg.GetSeverity() != nil {
			extraContent = append(extraContent, msg.GetSeverity()...)
		} else {
			extraContent = append(extraContent, config.SevInfo...)
		}

		// Protocol version
		extraContent = append(extraContent, '0')
		extraContent = append(extraContent, ' ')

		// Timestamp
		extraContent = time.Now().UTC().AppendFormat(extraContent, config.DateFormat)
		extraContent = append(extraContent, ' ')

		extraContent = append(extraContent, []byte(getHostname())...)
		extraContent = append(extraContent, ' ')

		// Service
		service := msg.GetOrigin().LogSource.Config.Service
		if service != "" {
			extraContent = append(extraContent, []byte(service)...)
		} else {
			extraContent = append(extraContent, '-')
		}

		// Extra
		extraContent = append(extraContent, []byte(" - - ")...)

		// Tags
		extraContent = append(extraContent, msg.GetOrigin().TagsPayload()...)
		extraContent = append(extraContent, ' ')

		return append(extraContent, redactedMsg...), nil

	}

	return redactedMsg, nil
}

func (r *raw) isRFC5424Formatted(content []byte) bool {
	// RFC2424 formatted messages start with `<%pri%>%protocol-version% `
	// pri is 1 to 3 digits, protocol-version is one digit (won't realisticly
	// be more before we kill this custom code)
	// As a result, the start is between 5 and 7 chars.
	if len(content) < 8 { // even is start could be only 5 chars, RFC5424 must have other chars like `-`
		return false
	}
	return rfc5424Pattern.Match(content[:8])
}

type proto struct{}

func (p *proto) encode(msg message.Message, redactedMsg []byte) ([]byte, error) {

	// TODO Remove occurrences of "severity" (it is now "status")
	// Compute the status
	var status string
	if msg.GetSeverity() != nil && bytes.Equal(msg.GetSeverity(), config.SevError) {
		status = config.StatusError
	} else {
		status = config.StatusInfo
	}

	return (&pb.Log{
		Message:   string(redactedMsg),
		Status:    status,
		Timestamp: time.Now().UTC().UnixNano(),
		Hostname:  getHostname(),
		Service:   msg.GetOrigin().LogSource.Config.Service,
		Source:    msg.GetOrigin().LogSource.Config.Source,
		Tags:      msg.GetOrigin().Tags(),
	}).Marshal()
}

// getHostname returns the hostname for the agent.
func getHostname() string {
	// Compute the hostname
	hostname, err := util.GetHostname()
	if err != nil {
		// this scenario is not likely to happen since the agent can not start without a hostname
		hostname = "unknown"
	}
	return hostname
}
