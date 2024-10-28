// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestTruncateString(t *testing.T) {
	assert.Equal(t, "", TruncateUTF8("", 5))
	assert.Equal(t, "tél", TruncateUTF8("télé", 5))
	assert.Equal(t, "t", TruncateUTF8("télé", 2))
	assert.Equal(t, "éé", TruncateUTF8("ééééé", 5))
	assert.Equal(t, "ééééé", TruncateUTF8("ééééé", 18))
	assert.Equal(t, "ééééé", TruncateUTF8("ééééé", 10))
	assert.Equal(t, "ééé", TruncateUTF8("ééééé", 6))
	assert.Equal(t, "", TruncateUTF8("肋", 2))
	// Testing 4 character split
	assert.Equal(t, "🠠", TruncateUTF8("🠠a", 4))
	// Testing 3 character split
	assert.Equal(t, "⃠⃠", TruncateUTF8("⃠⃠a", 6))
	// Testing 2 character split
	assert.Equal(t, "à", TruncateUTF8("àa", 2))
}

func FuzzTruncateString(f *testing.F) {
	f.Add("télé", 5)
	f.Add("肋", 2)
	f.Fuzz(func(t *testing.T, s string, limit int) {
		if !utf8.Valid([]byte(s)) || limit <= 0 { // This function previously assumed these invariants so let's keep them for fuzzing.
			t.Skip()
		}
		result := TruncateUTF8(s, limit)
		if len(result) > limit {
			t.Errorf("%s was truncated to %s which is %d long. Longer than limit %d", s, result, len(result), limit)
		}
		assert.True(t, utf8.Valid([]byte(result)), "%s became invalid utf8 %s", s, result)
	})
}

func BenchmarkTruncateString(b *testing.B) {
	s := strings.Repeat("télé", 100)
	for i := 0; i < b.N; i++ {
		TruncateUTF8(s, 100)
	}
}
