// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"fmt"
	"strings"
	"testing"
)

func TestBin_incrSafe(t *testing.T) {
	const maxn = maxBinWidth
	tests := []struct {
		n            uint16
		by           int
		wantN        uint16
		wantOverflow int
		name         string
	}{
		{by: 1, wantN: 1},
		{n: 1, by: 1, wantN: 2},
		{n: maxn, by: 1, wantN: maxn, wantOverflow: 1},
		{by: maxn, wantN: maxn},
		{n: 1, by: maxn, wantN: maxn, wantOverflow: 1},
		{n: 100, by: 3 * maxn, wantN: maxn, wantOverflow: 2*maxn + 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			var (
				b           = bin{n: tt.n}
				gotOverflow = b.incrSafe(tt.by)
				errs        []string
				ok          = true
			)

			if tt.wantOverflow != gotOverflow {
				ok = false
				errs = append(errs, fmt.Sprintf("\toverflow: got %d, want %d",
					gotOverflow, tt.wantOverflow))
			}

			if tt.wantN != b.n {
				ok = false
				errs = append(errs, fmt.Sprintf("\tn: got %d, want %d",
					b.n, tt.wantN))
			}

			if ok {
				return
			}

			t.Errorf("Bin{n:%d}.tryIncr(%d) = %d\n%s",
				tt.n, tt.by, gotOverflow, strings.Join(errs, "\n"))

		})
	}
}

// TestAppendSafeCountValueDrivesBinCount shows why a large count is a memory
// hazard: appendSafe splits it into ceil(n/maxBinWidth) bins, whatever the number
// of keys. n stays under 1<<31, as int is 32 bits wide on some build targets.
func TestAppendSafeCountValueDrivesBinCount(t *testing.T) {
	const (
		n = 1 << 30
		// 65535*16384 = 1073725440, leaving a remainder bin of 16384.
		wantBins = 16385
	)

	bins := appendSafe(nil, 42, n)

	if got := len(bins); got != wantBins {
		t.Errorf("appendSafe(nil, 42, %d) produced %d bins, want %d", n, got, wantBins)
	}

	if got := binList(bins).nSum(); got != n {
		t.Errorf("appendSafe(nil, 42, %d) holds a total count of %d, want %d", n, got, n)
	}
}
