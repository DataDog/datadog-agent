// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package wire

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServiceCheckStatus is a service check's wire status.
type ServiceCheckStatus int

const (
	ServiceCheckUnknown ServiceCheckStatus = iota
	ServiceCheckOK
	ServiceCheckWarning
	ServiceCheckCritical
)

// ServiceCheck contains parsed service-check fields before server enrichment.
type ServiceCheck struct {
	Name      string
	Status    ServiceCheckStatus
	Timestamp int64
	Hostname  string
	Message   string
	Tags      []string
	// localData is used for Origin Detection
	LocalData origindetection.LocalData
	// externalData is used for Origin Detection
	ExternalData origindetection.ExternalData
	// cardinality is used for Origin Detection
	Cardinality string
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
		return nil, errors.New("invalid dogstatsd service check name: empty name")
	}
	return rawName, nil
}

func parseServiceCheckStatus(rawStatus []byte) (ServiceCheckStatus, error) {
	switch {
	case bytes.Equal(rawStatus, rawServiceCheckStatusOk):
		return ServiceCheckOK, nil
	case bytes.Equal(rawStatus, rawServiceCheckStatusWarning):
		return ServiceCheckWarning, nil
	case bytes.Equal(rawStatus, rawServiceCheckStatusCritical):
		return ServiceCheckCritical, nil
	case bytes.Equal(rawStatus, rawServiceCheckStatusUnknown):
		return ServiceCheckUnknown, nil
	}
	return ServiceCheckUnknown, fmt.Errorf("invalid dogstatsd service check status: %q", rawStatus)
}

func parseServiceCheckTimestamp(rawTimestamp []byte) (int64, error) {
	return strconv.ParseInt(string(rawTimestamp), 10, 64)
}

func (p *Parser) applyServiceCheckOptionalField(serviceCheck ServiceCheck, optionalField []byte) (ServiceCheck, error) {
	newServiceCheck := serviceCheck
	var err error
	switch {
	case bytes.HasPrefix(optionalField, serviceCheckTimestampPrefix):
		newServiceCheck.Timestamp, err = parseServiceCheckTimestamp(optionalField[len(serviceCheckTimestampPrefix):])
	case bytes.HasPrefix(optionalField, serviceCheckHostnamePrefix):
		newServiceCheck.Hostname = string(optionalField[len(serviceCheckHostnamePrefix):])
	case bytes.HasPrefix(optionalField, serviceCheckTagsPrefix):
		newServiceCheck.Tags = p.parseTags(optionalField[len(serviceCheckTagsPrefix):])
	case bytes.HasPrefix(optionalField, serviceCheckMessagePrefix):
		newServiceCheck.Message = string(optionalField[len(serviceCheckMessagePrefix):])
	case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, localDataPrefix):
		newServiceCheck.LocalData = p.parseLocalData(optionalField[len(localDataPrefix):])
	case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, externalDataPrefix):
		newServiceCheck.ExternalData = p.parseExternalData(optionalField[len(externalDataPrefix):])
	case p.dsdOriginEnabled && bytes.HasPrefix(optionalField, cardinalityPrefix):
		newServiceCheck.Cardinality = string(optionalField[len(cardinalityPrefix):])
	}
	if err != nil {
		return serviceCheck, err
	}
	return newServiceCheck, nil
}

// ParseServiceCheck parses a service check, ignoring malformed optional fields.
func (p *Parser) ParseServiceCheck(message []byte) (ServiceCheck, error) {
	if !hasServiceCheckFormat(message) {
		return ServiceCheck{}, errors.New("invalid dogstatsd service check format")
	}
	// pop the _sc| header
	message = message[4:]

	rawName, message := nextField(message)
	name, err := parseServiceCheckName(rawName)
	if err != nil {
		return ServiceCheck{}, err
	}

	rawStatus, message := nextField(message)
	status, err := parseServiceCheckStatus(rawStatus)
	if err != nil {
		return ServiceCheck{}, err
	}

	serviceCheck := ServiceCheck{
		Name:   string(name),
		Status: status,
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
