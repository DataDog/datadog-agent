// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Point represents a metric value at a specific time
type Point struct {
	Ts    float64
	Value float64
}

// MarshalJSON return a Point as an array of value (to be compatible with v1 API)
// FIXME(maxime): to be removed when v2 endpoints are available
// Note: it is not used with jsoniter, encodePoints takes over
func (p *Point) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("[%v, %v]", int64(p.Ts), p.Value)), nil
}

// Serie holds a timeseries (w/ json serialization to DD API format)
type Serie struct {
	Name           string               `json:"metric"`
	Points         []Point              `json:"points"`
	Tags           tagset.CompositeTags `json:"tags"`
	Host           string               `json:"host"`
	Device         string               `json:"device,omitempty"` // FIXME(olivier): remove as soon as the v1 API can handle `device` as a regular tag
	MType          APIMetricType        `json:"type"`
	Interval       int64                `json:"interval"`
	SourceTypeName string               `json:"source_type_name,omitempty"`
	ContextKey     ckey.ContextKey      `json:"-"`
	NameSuffix     string               `json:"-"`
}

// SeriesAPIV2Enum returns the enumeration value for MetricPayload.MetricType in
// https://github.com/DataDog/agent-payload/blob/master/proto/metrics/agent_payload.proto
func (a APIMetricType) SeriesAPIV2Enum() int32 {
	switch a {
	case APIGaugeType:
		return 0
	case APIRateType:
		return 2
	case APICountType:
		return 1
	default:
		panic("unknown APIMetricType")
	}
}

// UnmarshalJSON is a custom unmarshaller for Point (used for testing)
func (p *Point) UnmarshalJSON(buf []byte) error {
	tmp := []interface{}{&p.Ts, &p.Value}
	wantLen := len(tmp)
	if err := json.Unmarshal(buf, &tmp); err != nil {
		return err
	}
	if len(tmp) != wantLen {
		return fmt.Errorf("wrong number of fields in Point: %d != %d", len(tmp), wantLen)
	}
	return nil
}

// String could be used for debug logging
func (e Serie) String() string {
	s, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return string(s)
}
