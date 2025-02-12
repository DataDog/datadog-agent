// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package ebpftest

import (
	"encoding/binary"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

// TestCgoAlignment checks if the provided type's size and amount of data necessary to read with binary.Read match.
// If they do not, that likely indicates there is a "hole" in the C definition of the type.
// https://lwn.net/Articles/335942/ has more details on what a "hole" is and how `pahole` can help.
func TestCgoAlignment[K any](t *testing.T) {
	var x K
	require.Equal(t, int(unsafe.Sizeof(x)), binary.Size(&x), "type %T has holes or size does not match between binary.Read and in-memory. Check 'pahole --show_reorg_steps --reorganize -C <c_type_name> <ebpf_object_file.o>' for a reorganized layout", x)
}
