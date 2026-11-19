// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package auditorimpl

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

// marshalWithEncodingJSON is the document encoding/json writes for registry.
func marshalWithEncodingJSON(t *testing.T, registry map[string]*RegistryEntry) []byte {
	t.Helper()
	r := JSONRegistry{Version: registryAPIVersion, Registry: make(map[string]RegistryEntry, len(registry))}
	for identifier, entry := range registry {
		r.Registry[identifier] = *entry
	}
	b, err := json.Marshal(r)
	require.NoError(t, err)
	return b
}

func TestAppendRegistryJSONMatchesEncodingJSONPerEntry(t *testing.T) {
	config := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               3,
		CountToSkip:         1,
		MaxBytes:            100000,
		Source:              types.FingerprintConfigSourcePerSource,
	}
	cases := map[string]*RegistryEntry{
		"file:/var/log/app.log": {
			LastUpdated: time.Date(2026, time.November, 19, 20, 4, 5, 123456789, time.UTC),
			Offset:      "4096",
			TailingMode: "beginning",
		},
		"file:/var/log/fingerprinted.log": {
			LastUpdated:        time.Date(2024, time.July, 18, 1, 1, 1, 0, time.UTC),
			Offset:             "150",
			TailingMode:        "end",
			IngestionTimestamp: 1795090682619000000,
			Fingerprint:        types.Fingerprint{Value: 18446744073709551615, Config: config},
		},
		"file:/var/log/<html>&\"quoted\"\\back.log": {
			LastUpdated: time.Date(2026, time.January, 2, 3, 4, 5, 0, time.FixedZone("EST", -5*3600)),
			Offset:      "tab\there\nnewline\x01ctl",
		},
		"file:/var/log/ünïcødé sep\xffbad.log": {
			Offset:      "-1",
			TailingMode: "forceEnd",
		},
		"journald:": {},
	}
	for identifier, entry := range cases {
		t.Run(fmt.Sprintf("%q", identifier), func(t *testing.T) {
			registry := map[string]*RegistryEntry{identifier: entry}
			got, err := appendRegistryJSON(nil, registry)
			require.NoError(t, err)
			assert.Equal(t, string(marshalWithEncodingJSON(t, registry)), string(got))
		})
	}
}

func TestAppendRegistryJSONEmptyRegistry(t *testing.T) {
	got, err := appendRegistryJSON(nil, map[string]*RegistryEntry{})
	require.NoError(t, err)
	assert.Equal(t, string(marshalWithEncodingJSON(t, map[string]*RegistryEntry{})), string(got))
}

func TestAppendRegistryJSONDecodesToSameRegistry(t *testing.T) {
	registry := make(map[string]*RegistryEntry)
	for i := 0; i < 200; i++ {
		entry := &RegistryEntry{
			LastUpdated:        time.Date(2026, time.November, 19, 0, 0, i, i*1000, time.UTC),
			Offset:             strconv.Itoa(i * 17),
			TailingMode:        "beginning",
			IngestionTimestamp: int64(i) * 1e9,
		}
		if i%2 == 0 {
			entry.Fingerprint = types.Fingerprint{Value: uint64(i), Config: &types.FingerprintConfig{
				FingerprintStrategy: types.FingerprintStrategyByteChecksum,
				MaxBytes:            i,
				Source:              types.FingerprintConfigSourceGlobal,
			}}
		}
		registry[fmt.Sprintf("file:/var/opt/apps/ims/log/msg2file/%06d.log", i)] = entry
	}

	got, err := appendRegistryJSON(nil, registry)
	require.NoError(t, err)

	var fromEncoder, fromEncodingJSON JSONRegistry
	require.NoError(t, json.Unmarshal(got, &fromEncoder))
	require.NoError(t, json.Unmarshal(marshalWithEncodingJSON(t, registry), &fromEncodingJSON))
	assert.Equal(t, fromEncodingJSON, fromEncoder)

	recovered, err := unmarshalRegistryV2(got)
	require.NoError(t, err)
	assert.Len(t, recovered, len(registry))
}

func TestAppendRegistryJSONReusesBuffer(t *testing.T) {
	registry := map[string]*RegistryEntry{"file:/var/log/app.log": {Offset: "1"}}
	first, err := appendRegistryJSON(nil, registry)
	require.NoError(t, err)
	second, err := appendRegistryJSON(first[:0], registry)
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
	assert.Equal(t, &first[:1][0], &second[:1][0], "encoding into a buffer with enough capacity reallocated it")
}

func TestAppendRegistryJSONRejectsTimesEncodingJSONRejects(t *testing.T) {
	registry := map[string]*RegistryEntry{"file:/var/log/app.log": {LastUpdated: time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)}}
	_, err := appendRegistryJSON(nil, registry)
	assert.Error(t, err)
}

// TestAppendRegistryJSONCoversEveryField fails when a field is added to a type
// the registry encoder writes field by field, so the encoder and
// TestAppendRegistryJSONMatchesEncodingJSONPerEntry are updated with it.
func TestAppendRegistryJSONCoversEveryField(t *testing.T) {
	assert.Equal(t, 5, reflect.TypeOf(RegistryEntry{}).NumField(), "update appendRegistryEntryJSON for the new RegistryEntry field")
	assert.Equal(t, 2, reflect.TypeOf(types.Fingerprint{}).NumField(), "update appendRegistryEntryJSON for the new Fingerprint field")
	assert.Equal(t, 5, reflect.TypeOf(types.FingerprintConfig{}).NumField(), "update appendFingerprintConfigJSON for the new FingerprintConfig field")
	assert.Equal(t, 2, reflect.TypeOf(JSONRegistry{}).NumField(), "update appendRegistryJSON for the new JSONRegistry field")
}

func BenchmarkRegistryEncoding(b *testing.B) {
	registry := make(map[string]*RegistryEntry, 220000)
	config := &types.FingerprintConfig{FingerprintStrategy: "disabled", MaxBytes: 100000, Source: types.FingerprintConfigSourceGlobal}
	now := time.Now().UTC()
	for i := 0; i < 220000; i++ {
		registry[fmt.Sprintf("file:/var/opt/apps/ims/log/msg2file/2611%02d/MQSI_GXSIMS02_2611%09d.log", i%30, i)] = &RegistryEntry{
			LastUpdated: now, Offset: strconv.Itoa(i), TailingMode: "beginning", IngestionTimestamp: int64(i),
			Fingerprint: types.Fingerprint{Config: config},
		}
	}
	b.Run("encoding_json", func(b *testing.B) {
		for b.Loop() {
			r := JSONRegistry{Version: registryAPIVersion, Registry: make(map[string]RegistryEntry, len(registry))}
			for identifier, entry := range registry {
				r.Registry[identifier] = *entry
			}
			if _, err := json.Marshal(r); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("appendRegistryJSON", func(b *testing.B) {
		var buf []byte
		for b.Loop() {
			var err error
			if buf, err = appendRegistryJSON(buf[:0], registry); err != nil {
				b.Fatal(err)
			}
		}
	})
}
