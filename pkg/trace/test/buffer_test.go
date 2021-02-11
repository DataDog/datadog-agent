// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package test

import (
	"testing"
)

func TestSafeBuffer(t *testing.T) {
	sb := newSafeBufferWithSize(10)
	for i, tt := range []struct {
		in  string
		out string
	}{
		{"12345", "12345"},
		{"67", "1234567"},
		{"123456", "4567123456"},
		{"789", "7123456789"},
		{"abcdefg", "789abcdefg"},
		{"abcdefghij", "abcdefghij"},
		{"abcdefghijklmnop", "ghijklmnop"},
	} {
		n, err := sb.Write([]byte(tt.in))
		if err != nil {
			t.Fatal(err)
		}
		if n != len(tt.in) {
			t.Fatalf("wrote %d instead of %d on step %d", n, len(tt.in), i)
		}
		if sb.String() != tt.out {
			t.Fatalf("got %q, wanted %q", sb.String(), tt.out)
		}
	}

}
