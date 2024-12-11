// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"bytes"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
	// externalData is used for Origin Detection
	externalData string
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
	if message == nil {
		return false
	}
	separatorCount := bytes.Count(message, fieldSeparator)
	if separatorCount < 2 {
		return false
	}
	if len(message) < 4 {
		return false
	}
	return true
}

func parseServiceCheckName(rawName []byte) ([]byte, error) {
	if len(rawName) == 0 {
		return nil, fmt.Errorf("invalid dogstatsd service check name: empty name")
	}
	return rawName, nil
}

func parseServiceCheckStatus(rawStatus []byte) (serviceCheckStatus, error) {
	switch {
	case bytes.Equal(rawStatus, rawServiceCheckStatusOk):
		return serviceCheckStatusOk, nil
	case bytes.Equal(rawStatus, rawServiceCheckStatusWarning):
		return serviceCheckStatusWarning, nil
	case bytes.Equal(rawStatus, rawServiceCheckStatusCritical):
		return serviceCheckStatusCritical, nil
	case bytes.Equal(rawStatus, rawServiceCheckStatusUnknown):
		return serviceCheckStatusUnknown, nil
	}
	return serviceCheckStatusUnknown, fmt.Errorf("invalid dogstatsd service check status: %q", rawStatus)
}

func parseServiceCheckTimestamp(rawTimestamp []byte) (int64, error) {
	return strconv.ParseInt(string(rawTimestamp), 10, 64)
}

func (p *parser) applyServiceCheckOptionalField(serviceCheck dogstatsdServiceCheck, optionalField []byte) (dogstatsdServiceCheck, error) {
	newServiceCheck := serviceCheck
	var err error
	switch {
	case bytes.HasPrefix(optionalField, serviceCheckTimestampPrefix):
		newServiceCheck.timestamp, err = parseServiceCheckTimestamp(optionalField[len(serviceCheckTimestampPrefix):])
	case bytes.HasPrefix(optionalField, serviceCheckHostnamePrefix):
		newServiceCheck.hostname = string(optionalField[len(serviceCheckHostnamePrefix):])
	case bytes.HasPrefix(optionalField, serviceCheckTagsPrefix):
		newServiceCheck.tags = p.parseTags(optionalField[len(serviceCheckTagsPrefix):])
	case bytes.HasPrefix(optionalField, serviceCheckMessagePrefix):
		newServiceCheck.message = string(optionalField[len(serviceCheckMessagePrefix):])
	case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, localDataPrefix):
		newServiceCheck.containerID = p.resolveContainerIDFromLocalData(optionalField)
	case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, externalDataPrefix):
		newServiceCheck.externalData = string(optionalField[len(externalDataPrefix):])
	}
	if err != nil {
		return serviceCheck, err
	}
	return newServiceCheck, nil
}

func (p *parser) parseServiceCheck(message []byte) (dogstatsdServiceCheck, error) {
	if !hasServiceCheckFormat(message) {
		return dogstatsdServiceCheck{}, fmt.Errorf("invalid dogstatsd service check format")
	}
	// pop the _sc| header
	message = message[4:]

	rawName, message := nextField(message)
	name, err := parseServiceCheckName(rawName)
	if err != nil {
		return dogstatsdServiceCheck{}, err
	}

	rawStatus, message := nextField(message)
	status, err := parseServiceCheckStatus(rawStatus)
	if err != nil {
		return dogstatsdServiceCheck{}, err
	}

	serviceCheck := dogstatsdServiceCheck{
		name:   string(name),
		status: status,
	}

	var optionalField []byte
	for message != nil {
		optionalField, message = nextField(message)
		serviceCheck, err = p.applyServiceCheckOptionalField(serviceCheck, optionalField)
		if err != nil {
			log.Warnf("invalid service check optional field: %v", err)
		}
	}
	return serviceCheck, nil
}
