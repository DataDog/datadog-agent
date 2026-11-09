// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package idx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinylib/msgp/msgp"
)

// TestParseStringBytesRef covers the branches of parseStringBytesRef, which interns
// directly from the msgpack bytes rather than allocating a string first. These
// branches (nil, bin, invalid UTF-8) were previously uncovered.
func TestParseStringBytesRef(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"str", msgp.AppendString(nil, "http.method"), "http.method"},
		{"empty str", msgp.AppendString(nil, ""), ""},
		{"nil", msgp.AppendNil(nil), ""},
		{"bin", msgp.AppendBytes(nil, []byte("binary-value")), "binary-value"},
		// Lone continuation byte: repaired to the replacement character.
		{"invalid utf8", msgp.AppendString(nil, "bad\x80value"), "bad�value"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := NewStringTable()
			ref, rest, err := parseStringBytesRef(st, tc.in)
			require.NoError(t, err)
			assert.Empty(t, rest, "entire input should be consumed")
			assert.Equal(t, tc.want, st.Get(ref))
		})
	}

	t.Run("type error", func(t *testing.T) {
		st := NewStringTable()
		_, _, err := parseStringBytesRef(st, msgp.AppendInt64(nil, 42))
		assert.Error(t, err)
	})
}

// TestParseStringBytesRefDedup verifies that repeated values resolve to the same
// reference — the property the allocation-free intern path relies on.
func TestParseStringBytesRefDedup(t *testing.T) {
	st := NewStringTable()
	var bts []byte
	for range 3 {
		bts = msgp.AppendString(bts, "env")
		bts = msgp.AppendString(bts, "production")
	}

	var refs []uint32
	rest := bts
	for range 6 {
		var ref uint32
		var err error
		ref, rest, err = parseStringBytesRef(st, rest)
		require.NoError(t, err)
		refs = append(refs, ref)
	}

	assert.Equal(t, []uint32{refs[0], refs[1], refs[0], refs[1], refs[0], refs[1]}, refs)
	// "", "env", "production"
	assert.Equal(t, 3, st.Len())
}

// TestAddBytesDoesNotAliasInput guards the one real hazard of interning from bytes:
// the table must copy on a miss, never retain the caller's buffer. In production
// that buffer is a pooled HTTP body that is reused by the next request.
func TestAddBytesDoesNotAliasInput(t *testing.T) {
	st := NewStringTable()
	buf := []byte("service-name")

	ref := st.AddBytes(buf)
	require.Equal(t, "service-name", st.Get(ref))

	copy(buf, "OVERWRITTEN!")
	assert.Equal(t, "service-name", st.Get(ref), "string table must not alias the input buffer")

	// The overwritten bytes are a genuinely new value, not a false hit.
	ref2 := st.AddBytes(buf)
	assert.NotEqual(t, ref, ref2)
	assert.Equal(t, "OVERWRITTEN!", st.Get(ref2))
}

// TestAddBytesMatchesAdd pins AddBytes to the semantics of Add.
func TestAddBytesMatchesAdd(t *testing.T) {
	values := []string{"", "env", "production", "env", "a", "production"}

	viaAdd := NewStringTable()
	viaBytes := NewStringTable()
	for _, v := range values {
		assert.Equal(t, viaAdd.Add(v), viaBytes.AddBytes([]byte(v)), "value %q", v)
	}
	assert.Equal(t, viaAdd.Len(), viaBytes.Len())
}
