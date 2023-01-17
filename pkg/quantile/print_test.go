// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package quantile

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrintBins(t *testing.T) {
	mb := func(dsl string) []bin {
		s := ParseSketch(t, dsl)
		return s.bins
	}

	b10 := "0:1 1:1 2:1 3:1 4:1 5:1 6:1 7:1 8:1 9:1"

	for _, tt := range []struct {
		bins []bin
		w    int
		exp  string
	}{
		{
			bins: mb(b10),
			exp:  b10,
		},
		{
			bins: mb(b10),
			exp:  b10,
			w:    10,
		},
		{
			bins: mb(b10),
			exp:  strings.Replace(b10, " ", "\n", -1),
			w:    1,
		},
		{
			bins: mb(b10),
			exp:  "0:1 1:1\n2:1 3:1\n4:1 5:1\n6:1 7:1\n8:1 9:1",
			w:    2,
		},
		{
			bins: mb(b10),
			exp:  "0:1 1:1 2:1\n3:1 4:1 5:1\n6:1 7:1 8:1\n9:1",
			w:    3,
		},
		{
			bins: mb(b10),
			exp:  "0:1 1:1 2:1 3:1 4:1 5:1 6:1 7:1 8:1\n9:1",
			w:    9,
		},
	} {
		name := fmt.Sprintf("w=%d", tt.w)
		t.Run(name, func(t *testing.T) {
			var b strings.Builder
			printBins(&b, tt.bins, tt.w)

			got := b.String()
			require.Equal(t, tt.exp, got)
		})
	}

	t.Run("demo", func(t *testing.T) {
		var bins binList
		for i := 0; i < defaultBinPerLine*2+1; i++ {
			bins = append(bins, bin{k: Key(i), n: 1})
		}

		s := bins.String()
		lines := strings.Split(s, "\n")
		require.Equal(t, []string{
			"0:1 1:1 2:1 3:1 4:1 5:1 6:1 7:1 8:1 9:1 10:1 11:1 12:1 13:1 14:1 15:1 16:1 17:1 18:1 19:1 20:1 21:1 22:1 23:1 24:1 25:1 26:1 27:1 28:1 29:1 30:1 31:1",
			"32:1 33:1 34:1 35:1 36:1 37:1 38:1 39:1 40:1 41:1 42:1 43:1 44:1 45:1 46:1 47:1 48:1 49:1 50:1 51:1 52:1 53:1 54:1 55:1 56:1 57:1 58:1 59:1 60:1 61:1 62:1 63:1",
			"64:1",
		}, lines)

	})
}
