// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ebpftest

import (
	"bytes"
	"encoding/binary"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

// TestCgoAlignment checks if the provided type's size and amount of data necessary to read with binary.Read match.
// If they do not, that likely indicates there is a "hole" in the C definition of the type.
func TestCgoAlignment[K any](t *testing.T) {
	var x K
	rdr := bytes.NewReader(make([]byte, unsafe.Sizeof(x)))
	err := binary.Read(rdr, binary.NativeEndian, &x)
	require.NoError(t, err)
	require.Zero(t, rdr.Len(), "type %%T has holes or size does match between C and Go. Check 'pahole -C <c_type_name> <ebpf_object_file.o>' for layout", x)
}
