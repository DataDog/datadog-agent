// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package servicecheck

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
)

// ServiceCheckStatus represents the status associated with a service check
//
//nolint:revive // TODO(AML) Fix revive linter
type ServiceCheckStatus int

// Enumeration of the existing service check statuses, and their values
const (
	ServiceCheckOK       ServiceCheckStatus = iota
	ServiceCheckWarning  ServiceCheckStatus = 1
	ServiceCheckCritical ServiceCheckStatus = 2
	ServiceCheckUnknown  ServiceCheckStatus = 3
)

// String returns a string representation of ServiceCheckStatus
func (s ServiceCheckStatus) String() string {
	switch s {
	case ServiceCheckOK:
		return "OK"
	case ServiceCheckWarning:
		return "WARNING"
	case ServiceCheckCritical:
		return "CRITICAL"
	case ServiceCheckUnknown:
		return "UNKNOWN"
	default:
		return ""
	}
}

// ServiceCheck holds a service check (w/ serialization to DD api format)
type ServiceCheck struct {
	CheckName  string                 `json:"check"`
	Host       string                 `json:"host_name"`
	Ts         int64                  `json:"timestamp"`
	Status     ServiceCheckStatus     `json:"status"`
	Message    string                 `json:"message"`
	Tags       []string               `json:"tags"`
	OriginInfo taggertypes.OriginInfo `json:"-"` // OriginInfo is not serialized, it's used for origin detection
}

func (sc ServiceCheck) String() string {
	s, err := json.Marshal(sc)
	if err != nil {
		return ""
	}
	return string(s)
}

// ServiceChecks is a collection of ServiceCheck
type ServiceChecks []*ServiceCheck

// MarshalStrings converts the service checks to a sorted slice of string slices
func (sc ServiceChecks) MarshalStrings() ([]string, [][]string) {
	var headers = []string{"Check", "Hostname", "Timestamp", "Status", "Message", "Tags"}
	var payload = make([][]string, len(sc))

	for _, c := range sc {
		payload = append(payload, []string{
			c.CheckName,
			c.Host,
			strconv.FormatInt(c.Ts, 10),
			c.Status.String(),
			c.Message,
			strings.Join(c.Tags, ", "),
		})
	}

	sort.Slice(payload, func(i, j int) bool {
		// edge cases
		if len(payload[i]) == 0 && len(payload[j]) == 0 {
			return false
		}
		if len(payload[i]) == 0 || len(payload[j]) == 0 {
			return len(payload[i]) == 0
		}
		// sort by service check name
		if payload[i][0] != payload[j][0] {
			return payload[i][0] < payload[j][0]
		}
		// then by timestamp
		if payload[i][2] != payload[j][2] {
			return payload[i][2] < payload[j][2]
		}
		// finally by tags (last field) as tie breaker
		return payload[i][len(payload[i])-1] < payload[j][len(payload[j])-1]
	})

	return headers, payload
}
