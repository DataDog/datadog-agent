// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procmgrdecision

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRecord is a RecordReader that counts its calls.
type fakeRecord struct {
	enabled bool
	found   bool
	err     error
	calls   int
}

func (f *fakeRecord) Read() (bool, bool, error) {
	f.calls++
	return f.enabled, f.found, f.err
}

// fakeSeed is a Seed that counts its calls.
type fakeSeed struct {
	res   SeedResult
	err   error
	calls int
}

func (f *fakeSeed) seed() (SeedResult, error) {
	f.calls++
	return f.res, f.err
}

func boolPtr(b bool) *bool { return &b }

func TestResolveExplicitWins(t *testing.T) {
	tests := []struct {
		name  string
		value bool
	}{
		{name: "true", value: true},
		{name: "false", value: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A record that would answer, and a seed that would answer, must not be asked.
			rec := &fakeRecord{enabled: !tt.value, found: true}
			seed := &fakeSeed{res: SeedResult{Found: true, Enabled: !tt.value}}

			d, err := Resolve(boolPtr(tt.value), rec, seed.seed)

			require.NoError(t, err)
			assert.Equal(t, Decision{Enabled: tt.value, Source: SourceExplicit}, d)
			assert.Zero(t, rec.calls)
			assert.Zero(t, seed.calls)
		})
	}
}

func TestResolveExplicitIgnoresBrokenRecord(t *testing.T) {
	rec := &fakeRecord{err: ErrUntrusted}
	seed := &fakeSeed{err: &SeedError{Detail: "x", Err: errors.New("broken")}}

	d, err := Resolve(boolPtr(false), rec, seed.seed)

	require.NoError(t, err)
	assert.Equal(t, Decision{Enabled: false, Source: SourceExplicit}, d)
	assert.Zero(t, rec.calls)
	assert.Zero(t, seed.calls)
}

func TestResolveRecordWinsOverSeed(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{name: "record enabled", enabled: true},
		{name: "record disabled", enabled: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &fakeRecord{enabled: tt.enabled, found: true}
			// The seed says the opposite, and must not be consulted.
			seed := &fakeSeed{res: SeedResult{Found: true, Enabled: !tt.enabled, Detail: "/etc/x"}}

			d, err := Resolve(nil, rec, seed.seed)

			require.NoError(t, err)
			assert.Equal(t, Decision{Enabled: tt.enabled, Source: SourceRecord}, d)
			assert.Equal(t, 1, rec.calls)
			assert.Zero(t, seed.calls)
		})
	}
}

func TestResolveSeedWhenNoRecord(t *testing.T) {
	tests := []struct {
		name string
		res  SeedResult
		want Decision
	}{
		{
			name: "seed disabled with detail and warning",
			res:  SeedResult{Found: true, Enabled: false, Detail: "/etc/default/datadog-agent-procmgr", Warning: "w"},
			want: Decision{Enabled: false, Source: SourceSeed, Detail: "/etc/default/datadog-agent-procmgr", Warning: "w"},
		},
		{
			name: "seed enabled",
			res:  SeedResult{Found: true, Enabled: true, Detail: "/etc/x"},
			want: Decision{Enabled: true, Source: SourceSeed, Detail: "/etc/x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &fakeRecord{found: false}
			seed := &fakeSeed{res: tt.res}

			d, err := Resolve(nil, rec, seed.seed)

			require.NoError(t, err)
			assert.Equal(t, tt.want, d)
			assert.Equal(t, 1, rec.calls)
			assert.Equal(t, 1, seed.calls)
		})
	}
}

func TestResolveDefaultWhenNothingApplies(t *testing.T) {
	rec := &fakeRecord{found: false}
	seed := &fakeSeed{res: SeedResult{Found: false, Detail: "/etc/x"}}

	d, err := Resolve(nil, rec, seed.seed)

	require.NoError(t, err)
	assert.Equal(t, Decision{Enabled: true, Source: SourceDefault}, d)
	assert.Equal(t, Default, d.Enabled)
	assert.Equal(t, 1, rec.calls)
	assert.Equal(t, 1, seed.calls)
}

func TestResolveRecordErrorDoesNotFallThrough(t *testing.T) {
	tests := []struct {
		name string
		err  error
		is   error
	}{
		{name: "untrusted", err: ErrUntrusted, is: ErrUntrusted},
		{name: "invalid", err: ErrInvalid, is: ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &fakeRecord{err: tt.err}
			seed := &fakeSeed{res: SeedResult{Found: true, Enabled: false}}

			d, err := Resolve(nil, rec, seed.seed)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.is)
			assert.Equal(t, Decision{}, d)
			assert.Zero(t, seed.calls, "the seed must not be read after a record error")
		})
	}
}

func TestResolveSeedErrorDoesNotFallThrough(t *testing.T) {
	seedErr := &SeedError{Detail: "/etc/default/datadog-agent-procmgr", Err: errors.New("not a simple assignment")}
	rec := &fakeRecord{found: false}
	seed := &fakeSeed{err: seedErr}

	d, err := Resolve(nil, rec, seed.seed)

	require.Error(t, err)
	var got *SeedError
	require.ErrorAs(t, err, &got)
	assert.Equal(t, "/etc/default/datadog-agent-procmgr", got.Detail)
	assert.Equal(t, Decision{}, d)
}

func TestResolveNilRecordAndNilSeed(t *testing.T) {
	d, err := Resolve(nil, nil, nil)

	require.NoError(t, err)
	assert.Equal(t, Decision{Enabled: Default, Source: SourceDefault}, d)
}

func TestResolveNilSeedSkipsSeed(t *testing.T) {
	rec := &fakeRecord{found: true, enabled: false}

	d, err := Resolve(nil, rec, nil)

	require.NoError(t, err)
	assert.Equal(t, Decision{Enabled: false, Source: SourceRecord}, d)
}
