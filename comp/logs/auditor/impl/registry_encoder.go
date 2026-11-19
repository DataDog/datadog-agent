// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package auditorimpl

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

// appendRegistryJSON appends the on-disk form of registry to buf: the same
// document encoding/json produces for a JSONRegistry, field for field, with
// the registry's entries in map order rather than sorted by key. Readers
// decode it with encoding/json, which does not depend on key order.
//
// Encoding each field directly skips the reflection and the key sort that
// json.Marshal spends most of its time on for a registry of many entries.
func appendRegistryJSON(buf []byte, registry map[string]*RegistryEntry) ([]byte, error) {
	buf = append(buf, `{"Version":`...)
	buf = strconv.AppendInt(buf, registryAPIVersion, 10)
	buf = append(buf, `,"Registry":{`...)
	first := true
	for identifier, entry := range registry {
		if !first {
			buf = append(buf, ',')
		}
		first = false
		buf = appendJSONString(buf, identifier)
		buf = append(buf, ':')
		var err error
		if buf, err = appendRegistryEntryJSON(buf, entry); err != nil {
			return nil, err
		}
	}
	return append(buf, "}}"...), nil
}

func appendRegistryEntryJSON(buf []byte, entry *RegistryEntry) ([]byte, error) {
	buf = append(buf, `{"LastUpdated":`...)
	var err error
	if buf, err = appendJSONTime(buf, entry.LastUpdated); err != nil {
		return nil, err
	}
	buf = append(buf, `,"Offset":`...)
	buf = appendJSONString(buf, entry.Offset)
	buf = append(buf, `,"TailingMode":`...)
	buf = appendJSONString(buf, entry.TailingMode)
	buf = append(buf, `,"IngestionTimestamp":`...)
	buf = strconv.AppendInt(buf, entry.IngestionTimestamp, 10)
	buf = append(buf, `,"Fingerprint":{"Value":`...)
	buf = strconv.AppendUint(buf, entry.Fingerprint.Value, 10)
	buf = append(buf, `,"Config":`...)
	buf = appendFingerprintConfigJSON(buf, entry.Fingerprint.Config)
	return append(buf, "}}"...), nil
}

func appendFingerprintConfigJSON(buf []byte, config *types.FingerprintConfig) []byte {
	if config == nil {
		return append(buf, "null"...)
	}
	buf = append(buf, `{"fingerprint_strategy":`...)
	buf = appendJSONString(buf, string(config.FingerprintStrategy))
	buf = append(buf, `,"count":`...)
	buf = strconv.AppendInt(buf, int64(config.Count), 10)
	buf = append(buf, `,"count_to_skip":`...)
	buf = strconv.AppendInt(buf, int64(config.CountToSkip), 10)
	buf = append(buf, `,"max_bytes":`...)
	buf = strconv.AppendInt(buf, int64(config.MaxBytes), 10)
	buf = append(buf, `,"source":`...)
	buf = appendJSONString(buf, string(config.Source))
	return append(buf, '}')
}

// appendJSONTime appends t the way time.Time.MarshalJSON writes it. Times
// MarshalJSON rejects (years outside 0-9999) go through it to return its error.
func appendJSONTime(buf []byte, t time.Time) ([]byte, error) {
	if year := t.Year(); year < 0 || year > 9999 {
		b, err := t.MarshalJSON()
		if err != nil {
			return nil, err
		}
		return append(buf, b...), nil
	}
	buf = append(buf, '"')
	buf = t.AppendFormat(buf, time.RFC3339Nano)
	return append(buf, '"'), nil
}

// appendJSONString appends s as encoding/json writes a string. Printable ASCII
// with nothing encoding/json escapes is copied as is; any other string is
// handed to encoding/json so escaping matches it exactly.
func appendJSONString(buf []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c > 0x7e || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			b, _ := json.Marshal(s) // marshaling a string cannot fail
			return append(buf, b...)
		}
	}
	buf = append(buf, '"')
	buf = append(buf, s...)
	return append(buf, '"')
}
