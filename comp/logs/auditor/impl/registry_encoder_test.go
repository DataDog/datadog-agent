// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package auditorimpl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// encodeRegistryToBytes is what encodeRegistry writes for registry.
func encodeRegistryToBytes(t *testing.T, registry map[string]*RegistryEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	err := (&registryAuditor{registry: registry}).encodeRegistry(&buf)
	require.NoError(t, err)
	return buf.Bytes()
}

// manyEntries is a registry of n entries, every other one fingerprinted.
func manyEntries(n int) map[string]*RegistryEntry {
	registry := make(map[string]*RegistryEntry, n)
	for i := 0; i < n; i++ {
		entry := &RegistryEntry{
			LastUpdated:        time.Date(2026, time.November, 19, 0, 0, i%60, i*1000, time.UTC),
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
	return registry
}

func TestEncodeRegistryMatchesEncodingJSONPerEntry(t *testing.T) {
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
		`file:C:\logs\app "quoted".log`: {
			LastUpdated: time.Date(2026, time.March, 8, 2, 30, 0, 0, time.UTC),
			Offset:      `C:\offset\"q"`,
			TailingMode: "end",
		},
		"file:/var/log/del\x7f.log": {},
		"file:/var/log/zone.log": {
			LastUpdated: time.Date(2026, time.June, 1, 12, 0, 0, 5, time.FixedZone("", 23*3600+59*60)),
		},
	}
	for identifier, entry := range cases {
		t.Run(fmt.Sprintf("%q", identifier), func(t *testing.T) {
			registry := map[string]*RegistryEntry{identifier: entry}
			assert.Equal(t, string(marshalWithEncodingJSON(t, registry)), string(encodeRegistryToBytes(t, registry)))
		})
	}
}

func TestEncodeRegistryEmptyRegistry(t *testing.T) {
	registry := map[string]*RegistryEntry{}
	assert.Equal(t, string(marshalWithEncodingJSON(t, registry)), string(encodeRegistryToBytes(t, registry)))
}

func TestEncodeRegistryAcrossBatchesDecodesToSameRegistry(t *testing.T) {
	registry := manyEntries(5*registryEncodeBatchSize + 7)
	got := encodeRegistryToBytes(t, registry)

	var fromEncoder, fromEncodingJSON JSONRegistry
	require.NoError(t, json.Unmarshal(got, &fromEncoder))
	require.NoError(t, json.Unmarshal(marshalWithEncodingJSON(t, registry), &fromEncodingJSON))
	assert.Equal(t, fromEncodingJSON, fromEncoder)

	recovered, err := unmarshalRegistryV2(got)
	require.NoError(t, err)
	assert.Len(t, recovered, len(registry))
}

// removingWriter deletes entries from the auditor's registry on its first
// write, which carries the document header and comes after the identifiers
// are taken and before any entry is encoded.
type removingWriter struct {
	a       *registryAuditor
	remove  []string
	written bytes.Buffer
}

func (w *removingWriter) Write(p []byte) (int, error) {
	if w.remove != nil {
		w.a.registryMutex.Lock()
		for _, identifier := range w.remove {
			delete(w.a.registry, identifier)
		}
		w.a.registryMutex.Unlock()
		w.remove = nil
	}
	return w.written.Write(p)
}

// encodeRemovingAllBut encodes a registry of n entries while all but keep of
// them are removed, and returns the identifiers left and the registry written.
func encodeRemovingAllBut(t *testing.T, n, keep int) (map[string]*RegistryEntry, JSONRegistry) {
	t.Helper()
	registry := manyEntries(n)
	a := &registryAuditor{registry: registry}
	var remove []string
	for identifier := range registry {
		if len(remove) == n-keep {
			break
		}
		remove = append(remove, identifier)
	}
	w := &removingWriter{a: a, remove: remove}

	err := a.encodeRegistry(w)
	require.NoError(t, err)

	var written JSONRegistry
	require.NoError(t, json.Unmarshal(w.written.Bytes(), &written), "the registry written around removed entries is not valid JSON: %s", w.written.String())
	return registry, written
}

func TestEncodeRegistrySkipsEntriesRemovedDuringTheWrite(t *testing.T) {
	remaining, written := encodeRemovingAllBut(t, 3*registryEncodeBatchSize, 2*registryEncodeBatchSize)
	assert.Len(t, written.Registry, len(remaining), "entries removed before their batch was encoded were written")
	for identifier := range remaining {
		assert.Contains(t, written.Registry, identifier)
	}
}

// TestEncodeRegistryWithOnlyOneEntryLeft fails if skipping removed entries
// misplaces the separator before the first entry written. Three registries in
// random map order make it unlikely that the entry left is first in all of
// them.
func TestEncodeRegistryWithOnlyOneEntryLeft(t *testing.T) {
	for range 3 {
		remaining, written := encodeRemovingAllBut(t, 3*registryEncodeBatchSize, 1)
		assert.Len(t, written.Registry, 1)
		for identifier := range remaining {
			assert.Contains(t, written.Registry, identifier)
		}
	}
}

func TestEncodeRegistryWithEveryEntryRemoved(t *testing.T) {
	_, written := encodeRemovingAllBut(t, 2*registryEncodeBatchSize, 0)
	assert.Empty(t, written.Registry)
}

// lockCheckingWriter records whether registryMutex was free on every write.
type lockCheckingWriter struct {
	a          *registryAuditor
	writes     int
	writesHeld int
}

func (w *lockCheckingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.a.registryMutex.TryLock() {
		w.a.registryMutex.Unlock()
	} else {
		w.writesHeld++
	}
	return len(p), nil
}

func TestEncodeRegistryWritesWithoutHoldingTheLock(t *testing.T) {
	a := &registryAuditor{registry: manyEntries(3*registryEncodeBatchSize + 1)}
	w := &lockCheckingWriter{a: a}
	require.NoError(t, a.encodeRegistry(w))
	assert.Greater(t, w.writes, 4, "the registry was not written in batches")
	assert.Zero(t, w.writesHeld, "registryMutex was held during a write to the registry file")
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestEncodeRegistryReturnsWriteErrors(t *testing.T) {
	err := (&registryAuditor{registry: manyEntries(10)}).encodeRegistry(failingWriter{})
	assert.EqualError(t, err, "disk full")
}

func TestEncodeRegistryRejectsTimesEncodingJSONRejects(t *testing.T) {
	for name, lastUpdated := range map[string]time.Time{
		"year 10000":  time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC),
		"year -1":     time.Date(-1, time.January, 1, 0, 0, 0, 0, time.UTC),
		"zone +24:00": time.Date(2026, time.January, 1, 0, 0, 0, 0, time.FixedZone("", 24*3600)),
		"zone -25:00": time.Date(2026, time.January, 1, 0, 0, 0, 0, time.FixedZone("", -25*3600)),
	} {
		t.Run(name, func(t *testing.T) {
			_, wantErr := json.Marshal(lastUpdated)
			require.Error(t, wantErr, "encoding/json accepts this time")
			a := &registryAuditor{registry: map[string]*RegistryEntry{"file:/var/log/app.log": {LastUpdated: lastUpdated}}}
			err := a.encodeRegistry(io.Discard)
			assert.Error(t, err)
		})
	}
}

// TestEncodeRegistryCoversEveryField fails when a field is added to a type
// the registry encoder writes field by field, so the encoder and
// TestEncodeRegistryMatchesEncodingJSONPerEntry are updated with it.
func TestEncodeRegistryCoversEveryField(t *testing.T) {
	assert.Equal(t, 5, reflect.TypeOf(RegistryEntry{}).NumField(), "update appendRegistryEntryJSON for the new RegistryEntry field")
	assert.Equal(t, 2, reflect.TypeOf(types.Fingerprint{}).NumField(), "update appendRegistryEntryJSON for the new Fingerprint field")
	assert.Equal(t, 5, reflect.TypeOf(types.FingerprintConfig{}).NumField(), "update appendFingerprintConfigJSON for the new FingerprintConfig field")
	assert.Equal(t, 2, reflect.TypeOf(JSONRegistry{}).NumField(), "update encodeRegistry for the new JSONRegistry field")
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
	b.Run("encodeRegistry", func(b *testing.B) {
		a := &registryAuditor{registry: registry}
		for b.Loop() {
			if err := a.encodeRegistry(io.Discard); err != nil {
				b.Fatal(err)
			}
		}
	})
}
