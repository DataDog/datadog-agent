// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package helper

import (
	"expvar"

	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// ExpvarFields returns a map of all currently registered expvar key-value pairs.
// Each value is the JSON string returned by the expvar's String() method.
func ExpvarFields() map[string]string {
	fields := make(map[string]string)
	expvar.Do(func(kv expvar.KeyValue) {
		fields[kv.Key] = kv.Value.String()
	})
	return fields
}

// DefaultStatusResponse returns a GetStatusDetailsResponse populated with all in-process expvar data.
func DefaultStatusResponse() *pbcore.GetStatusDetailsResponse {
	return &pbcore.GetStatusDetailsResponse{
		MainSection: &pbcore.StatusSection{Fields: ExpvarFields()},
	}
}
