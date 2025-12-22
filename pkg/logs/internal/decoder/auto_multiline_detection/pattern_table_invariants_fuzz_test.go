// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"testing"

	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

func FuzzPatternTableInvariants(f *testing.F) {
	f.Add([]byte("!@#$%^&*()"), []byte{0, 1, 2, 0, 1, 2}, 5)
	f.Add([]byte("abcdef"), []byte{2, 2, 2, 2}, 3)

	f.Fuzz(func(t *testing.T, payload []byte, labelBytes []byte, maxSize int) {
		if len(payload) == 0 || len(labelBytes) == 0 {
			return
		}

		if maxSize < 0 {
			maxSize = -maxSize
		}
		maxTableSize := maxSize%16 + 1

		pt := NewPatternTable(maxTableSize, 1.0, status.NewInfoRegistry())
		tokenizer := NewTokenizer(0)

		// Distinct punctuation tokens to reduce accidental matching in threshold=1.0 comparisons.
		prefixes := []byte("!@#$%^&*()_+-=:;/.,'\"`~{}[]\\")

		inserts := 0
		for i := 0; i < len(payload) && inserts < 64; i++ {
			prefix := prefixes[int(payload[i])%len(prefixes)]
			bodyLen := int(payload[(i+1)%len(payload)]%16) + 1
			msg := make([]byte, 0, 1+1+bodyLen)
			msg = append(msg, prefix, ' ')
			for j := 0; j < bodyLen; j++ {
				// Alternate between letters and digits to produce varied token streams.
				if (j^i)&1 == 0 {
					msg = append(msg, 'a'+payload[(i+j)%len(payload)]%26)
				} else {
					msg = append(msg, '0'+payload[(i+j)%len(payload)]%10)
				}
			}

			ts, idx := tokenizer.tokenize(msg)
			if len(ts) == 0 {
				continue
			}

			label := Label(labelBytes[inserts%len(labelBytes)] % 3)
			ctx := &messageContext{
				rawMessage:      msg,
				tokens:          ts,
				tokenIndicies:   idx,
				label:           label,
				labelAssignedBy: "fuzz",
			}

			pt.ProcessAndContinue(ctx)
			inserts++

			if got := len(pt.table); got > maxTableSize {
				t.Fatalf("pattern table grew past max size: got=%d max=%d", got, maxTableSize)
			}
			if pt.index != int64(inserts) {
				t.Fatalf("pattern table index mismatch: got=%d want=%d", pt.index, inserts)
			}
			// Table must be sorted by descending count.
			for r := 1; r < len(pt.table); r++ {
				if pt.table[r-1].count < pt.table[r].count {
					t.Fatalf("pattern table not sorted: row[%d]=%d row[%d]=%d", r-1, pt.table[r-1].count, r, pt.table[r].count)
				}
			}
			for r, row := range pt.table {
				if row == nil {
					t.Fatalf("nil row at %d", r)
				}
				if row.count <= 0 {
					t.Fatalf("non-positive count at %d: %d", r, row.count)
				}
				if row.lastIndex <= 0 || row.lastIndex > pt.index {
					t.Fatalf("invalid lastIndex at %d: %d (index=%d)", r, row.lastIndex, pt.index)
				}
			}
		}
	})
}

