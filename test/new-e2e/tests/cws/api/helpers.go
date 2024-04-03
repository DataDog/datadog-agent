// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"errors"

	"github.com/DataDog/datadog-api-client-go/api/v2/datadog"
	"github.com/mitchellh/mapstructure"
)

// ErrNoSignalFound is returned when no signal is found
var ErrNoSignalFound = errors.New("no signal found")

// GetSignal returns the last signal matching the query
func (c *Client) GetSignal(query string) (*datadog.SecurityMonitoringSignalAttributes, error) {
	resp, err := c.getSignals(query)
	if err != nil {
		return nil, err
	}
	if len(resp.Data) > 0 {
		return resp.Data[len(resp.Data)-1].Attributes, nil
	}
	return nil, ErrNoSignalFound
}

// GetAppRulesetLoadedEvent returns a ruleset loaded event
func (c *Client) GetAppRulesetLoadedEvent(query string) (*RulesetLoadedEvent, error) {
	log, err := c.getLastMatchingLog(query)
	if err != nil {
		return nil, err
	}
	var ruleset RulesetLoadedEvent
	err = mapstructure.Decode(log.Attributes, &ruleset)
	if err != nil {
		return nil, err
	}
	return &ruleset, nil
}

// GetAppRuleEvent returns a rule event
func (c *Client) GetAppRuleEvent(query string) (*RuleEvent, error) {
	log, err := c.getLastMatchingLog(query)
	if err != nil {
		return nil, err
	}
	var rule RuleEvent
	err = mapstructure.Decode(log.Attributes, &rule)
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// GetAppSelftestsEvent returns a selftests event
func (c *Client) GetAppSelftestsEvent(query string) (*SelftestsEvent, error) {
	log, err := c.getLastMatchingLog(query)
	if err != nil {
		return nil, err
	}
	var selftests SelftestsEvent
	err = mapstructure.Decode(log.Attributes, &selftests)
	if err != nil {
		return nil, err
	}
	return &selftests, nil
}
