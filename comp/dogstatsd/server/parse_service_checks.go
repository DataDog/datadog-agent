// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

type serviceCheckStatus int

const (
	serviceCheckStatusUnknown serviceCheckStatus = iota
	serviceCheckStatusOk
	serviceCheckStatusWarning
	serviceCheckStatusCritical
)

type dogstatsdServiceCheck struct {
	name      string
	status    serviceCheckStatus
	timestamp int64
	hostname  string
	message   string
	tags      []string
	// containerID represents the container ID of the sender (optional).
	containerID []byte
}

var (
	rawServiceCheckStatusOk       = []byte("0")
	rawServiceCheckStatusWarning  = []byte("1")
	rawServiceCheckStatusCritical = []byte("2")
	rawServiceCheckStatusUnknown  = []byte("3")

	serviceCheckTimestampPrefix = []byte("d:")
	serviceCheckHostnamePrefix  = []byte("h:")
	serviceCheckMessagePrefix   = []byte("m:")
	serviceCheckTagsPrefix      = []byte("#")
)

// sanity checks a given message against the metric sample format
func hasServiceCheckFormat(message []byte) bool {
	panic("not called")
}

func parseServiceCheckName(rawName []byte) ([]byte, error) {
	panic("not called")
}

func parseServiceCheckStatus(rawStatus []byte) (serviceCheckStatus, error) {
	panic("not called")
}

func parseServiceCheckTimestamp(rawTimestamp []byte) (int64, error) {
	panic("not called")
}

func (p *parser) applyServiceCheckOptionalField(serviceCheck dogstatsdServiceCheck, optionalField []byte) (dogstatsdServiceCheck, error) {
	panic("not called")
}

func (p *parser) parseServiceCheck(message []byte) (dogstatsdServiceCheck, error) {
	panic("not called")
}
