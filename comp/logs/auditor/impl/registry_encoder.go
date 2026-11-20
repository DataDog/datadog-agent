// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package auditorimpl

import (
	"encoding/json"
	"io"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

// registryEncodeBatchSize is the number of entries encoded per hold of
// registryMutex while the registry is written.
const registryEncodeBatchSize = 256

// encodeRegistry writes the on-disk form of the registry to w: the same
// document encoding/json produces for a JSONRegistry, field for field, with
// entries in map order rather than sorted by key. Readers decode it with
// encoding/json, which does not depend on key order. Encoding each field
// directly skips the reflection and the key sort that json.Marshal spends
// most of its time on for a registry of many entries.
//
// The identifiers are taken under registryMutex, then entries are encoded
// registryEncodeBatchSize at a time, each batch under registryMutex and each
// write to w outside it, so disk writes never hold the lock and the encoded
// registry is never held in memory whole. Entries removed after the
// identifiers are taken are left out; entries added after are written by the
// next flush.
func (a *registryAuditor) encodeRegistry(w io.Writer) error {
	a.registryMutex.Lock()
	identifiers := make([]string, 0, len(a.registry))
	for identifier := range a.registry {
		identifiers = append(identifiers, identifier)
	}
	a.registryMutex.Unlock()

	buf := append(make([]byte, 0, 512*registryEncodeBatchSize), `{"Version":`...)
	buf = strconv.AppendInt(buf, registryAPIVersion, 10)
	buf = append(buf, `,"Registry":{`...)
	if _, err := w.Write(buf); err != nil {
		return err
	}
	buf = buf[:0]
	first := true
	for start := 0; start < len(identifiers); start += registryEncodeBatchSize {
		batch := identifiers[start:min(start+registryEncodeBatchSize, len(identifiers))]
		a.registryMutex.Lock()
		for _, identifier := range batch {
			entry, ok := a.registry[identifier]
			if !ok {
				continue
			}
			if !first {
				buf = append(buf, ',')
			}
			first = false
			buf = appendJSONString(buf, identifier)
			buf = append(buf, ':')
			var err error
			if buf, err = appendRegistryEntryJSON(buf, entry); err != nil {
				a.registryMutex.Unlock()
				return err
			}
		}
		a.registryMutex.Unlock()
		if _, err := w.Write(buf); err != nil {
			return err
		}
		buf = buf[:0]
	}
	buf = append(buf, "}}"...)
	_, err := w.Write(buf)
	return err
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
// MarshalJSON rejects, years outside 0-9999 and zone offsets of a day or
// more, go through it to return its error.
func appendJSONTime(buf []byte, t time.Time) ([]byte, error) {
	_, offset := t.Zone()
	if year := t.Year(); year < 0 || year > 9999 || offset <= -24*3600 || offset >= 24*3600 {
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
// is copied with '"' and '\' backslash-escaped, as encoding/json escapes
// them; a string with any other byte encoding/json escapes or rewrites
// (control characters, '<', '>', '&', and everything outside ASCII) is
// handed to encoding/json so escaping matches it exactly.
func appendJSONString(buf []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c > 0x7e || c == '<' || c == '>' || c == '&' {
			b, _ := json.Marshal(s) // marshaling a string cannot fail
			return append(buf, b...)
		}
	}
	buf = append(buf, '"')
	start := 0
	for i := 0; i < len(s); i++ {
		if c := s[i]; c == '"' || c == '\\' {
			buf = append(buf, s[start:i]...)
			buf = append(buf, '\\', c)
			start = i + 1
		}
	}
	buf = append(buf, s[start:]...)
	return append(buf, '"')
}
