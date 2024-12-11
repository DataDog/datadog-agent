// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"encoding/json"
	"errors"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/mitchellh/mapstructure"
)

// ErrNoSignalFound is returned when no signal is found
var ErrNoSignalFound = errors.New("no signal found")

// GetSignal returns the last signal matching the query
func (c *Client) GetSignal(query string) (*datadogV2.SecurityMonitoringSignalAttributes, error) {
	resp, err := c.getSignals(query)
	if err != nil {
		return nil, err
	}
	if len(resp.Data) > 0 {
		return resp.Data[len(resp.Data)-1].Attributes, nil
	}
	return nil, ErrNoSignalFound
}

// GetterFromPointer is a type constraint for log-based events
type GetterFromPointer[T any, E any] interface {
	*T
	Get() E
}

// GetAppEvent returns the last event matching the query
func GetAppEvent[T any, PT GetterFromPointer[T, *Event]](c *Client, query string) (*T, error) {
	log, err := c.getLastMatchingLog(query)
	if err != nil {
		return nil, err
	}
	var e T
	err = mapstructure.Decode(log.Attributes, &e)
	if err != nil {
		return nil, err
	}
	ptr := PT(&e)
	ptr.Get().marshaler = func() ([]byte, error) {
		return json.Marshal(log.Attributes)
	}
	ptr.Get().Tags = log.Tags
	return &e, nil
}
