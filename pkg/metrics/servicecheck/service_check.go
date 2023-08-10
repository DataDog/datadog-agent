// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package servicecheck TODO comment
package servicecheck

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Status represents the status associated with a service check
type Status int

// Enumeration of the existing service check statuses, and their values
const (
	OK       Status = iota
	Warning  Status = 1
	Critical Status = 2
	Unknown  Status = 3
)

// GetStatus returns the Status from and integer value
func GetStatus(val int) (Status, error) {
	switch val {
	case int(OK):
		return OK, nil
	case int(Warning):
		return Warning, nil
	case int(Critical):
		return Critical, nil
	case int(Unknown):
		return Unknown, nil
	default:
		return Unknown, fmt.Errorf("invalid value for a Status")
	}
}

// String returns a string representation of Status
func (s Status) String() string {
	switch s {
	case OK:
		return "OK"
	case Warning:
		return "WARNING"
	case Critical:
		return "CRITICAL"
	case Unknown:
		return "UNKNOWN"
	default:
		return ""
	}
}

// ServiceCheck holds a service check (w/ serialization to DD api format)
type ServiceCheck struct {
	CheckName        string   `json:"check"`
	Host             string   `json:"host_name"`
	Ts               int64    `json:"timestamp"`
	Status           Status   `json:"status"`
	Message          string   `json:"message"`
	Tags             []string `json:"tags"`
	OriginFromUDS    string   `json:"-"`
	OriginFromClient string   `json:"-"`
	Cardinality      string   `json:"-"`
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
