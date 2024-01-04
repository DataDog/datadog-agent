// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"bytes"
	"io"
	"testing"
)

// DumpMapsTestHelper dumps the content of the given maps to the test log, handling errors if any.
func DumpMapsTestHelper(t *testing.T, dumpfunc func(io.Writer, ...string) error, maps ...string) {
	for _, m := range maps {
		var buffer bytes.Buffer

		t.Log("Dumping map", m)
		err := dumpfunc(&buffer, m)
		if err != nil {
			t.Logf("Error dumping map %s: %s", m, err)
		} else {
			t.Log(buffer.String())
		}
	}
}
