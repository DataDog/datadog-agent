// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/config"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

type testCase struct {
	name                   string
	binary                 string
	probes                 []config.Probe
	expectedDecodingOutput string
	expectedToPass         bool
}

func TestDyninst(t *testing.T) {
	testCases := []testCase{
		// Basic type tests
		{
			name:   "test_single_byte",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_byte",
					Where: &config.Where{
						MethodName: "main.test_single_byte",
					},
				},
			},
			expectedDecodingOutput: `{"x":97}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_rune",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_rune",
					Where: &config.Where{
						MethodName: "main.test_single_rune",
					},
				},
			},
			expectedDecodingOutput: `{"x":1}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_bool",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_bool",
					Where: &config.Where{
						MethodName: "main.test_single_bool",
					},
				},
			},
			expectedDecodingOutput: `{"x":true}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_int",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_int",
					Where: &config.Where{
						MethodName: "main.test_single_int",
					},
				},
			},
			expectedDecodingOutput: `{"x":-1512}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_int8",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_int8",
					Where: &config.Where{
						MethodName: "main.test_single_int8",
					},
				},
			},
			expectedDecodingOutput: `{"x":-8}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_int16",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_int16",
					Where: &config.Where{
						MethodName: "main.test_single_int16",
					},
				},
			},
			expectedDecodingOutput: `{"x":-16}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_int32",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_int32",
					Where: &config.Where{
						MethodName: "main.test_single_int32",
					},
				},
			},
			expectedDecodingOutput: `{"x":-32}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_int64",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_int64",
					Where: &config.Where{
						MethodName: "main.test_single_int64",
					},
				},
			},
			expectedDecodingOutput: `{"x":-64}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_uint",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_uint",
					Where: &config.Where{
						MethodName: "main.test_single_uint",
					},
				},
			},
			expectedDecodingOutput: `{"x":1512}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_uint8",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_uint8",
					Where: &config.Where{
						MethodName: "main.test_single_uint8",
					},
				},
			},
			expectedDecodingOutput: `{"x":8}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_uint16",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_uint16",
					Where: &config.Where{
						MethodName: "main.test_single_uint16",
					},
				},
			},
			expectedDecodingOutput: `{"x":16}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_uint32",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_uint32",
					Where: &config.Where{
						MethodName: "main.test_single_uint32",
					},
				},
			},
			expectedDecodingOutput: `{"x":32}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_uint64",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_uint64",
					Where: &config.Where{
						MethodName: "main.test_single_uint64",
					},
				},
			},
			expectedDecodingOutput: `{"x":64}`,
			expectedToPass:         true,
		},
		{
			name:   "test_single_float32",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_float32",
					Where: &config.Where{
						MethodName: "main.test_single_float32",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         true,
		},
		{
			name:   "test_single_float64",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_float64",
					Where: &config.Where{
						MethodName: "main.test_single_float64",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_type_alias",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_type_alias",
					Where: &config.Where{
						MethodName: "main.test_type_alias",
					},
				},
			},
			expectedDecodingOutput: `{"x":3}`,
			expectedToPass:         true,
		},

		// String tests
		{
			name:   "test_single_string",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_single_string",
					Where: &config.Where{
						MethodName: "main.test_single_string",
					},
				},
			},
			expectedDecodingOutput: `{"x":"abc"}`,
			expectedToPass:         true,
		},
		{
			name:   "test_three_strings",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_three_strings",
					Where: &config.Where{
						MethodName: "main.test_three_strings",
					},
				},
			},
			expectedDecodingOutput: `{"x":"abc","y":"def","z":"ghi"}`,
			expectedToPass:         true,
		},
		{
			name:   "test_three_strings_in_struct",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_three_strings_in_struct",
					Where: &config.Where{
						MethodName: "main.test_three_strings_in_struct",
					},
				},
			},
			expectedDecodingOutput: `{"a":{"a":"abc","b":"def","c":"ghi"}}`,
			expectedToPass:         true,
		},
		{
			name:   "test_three_strings_in_struct_pointer",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_three_strings_in_struct_pointer",
					Where: &config.Where{
						MethodName: "main.test_three_strings_in_struct_pointer",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_one_string_in_struct_pointer",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_one_string_in_struct_pointer",
					Where: &config.Where{
						MethodName: "main.test_one_string_in_struct_pointer",
					},
				},
			},
			expectedDecodingOutput: `{}`,
			expectedToPass:         false,
		},

		// Array tests
		{
			name:   "test_byte_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_byte_array",
					Where: &config.Where{
						MethodName: "main.test_byte_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_rune_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_rune_array",
					Where: &config.Where{
						MethodName: "main.test_rune_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_string_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_string_array",
					Where: &config.Where{
						MethodName: "main.test_string_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_bool_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_bool_array",
					Where: &config.Where{
						MethodName: "main.test_bool_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_int_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_int_array",
					Where: &config.Where{
						MethodName: "main.test_int_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_int8_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_int8_array",
					Where: &config.Where{
						MethodName: "main.test_int8_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_int16_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_int16_array",
					Where: &config.Where{
						MethodName: "main.test_int16_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_int32_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_int32_array",
					Where: &config.Where{
						MethodName: "main.test_int32_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_int64_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_int64_array",
					Where: &config.Where{
						MethodName: "main.test_int64_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_uint_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_uint_array",
					Where: &config.Where{
						MethodName: "main.test_uint_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_uint8_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_uint8_array",
					Where: &config.Where{
						MethodName: "main.test_uint8_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_uint16_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_uint16_array",
					Where: &config.Where{
						MethodName: "main.test_uint16_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_uint32_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_uint32_array",
					Where: &config.Where{
						MethodName: "main.test_uint32_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_uint64_array",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_uint64_array",
					Where: &config.Where{
						MethodName: "main.test_uint64_array",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_array_of_arrays",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_array_of_arrays",
					Where: &config.Where{
						MethodName: "main.test_array_of_arrays",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_array_of_strings",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_array_of_strings",
					Where: &config.Where{
						MethodName: "main.test_array_of_strings",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_array_of_arrays_of_arrays",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_array_of_arrays_of_arrays",
					Where: &config.Where{
						MethodName: "main.test_array_of_arrays_of_arrays",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_array_of_structs",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_array_of_structs",
					Where: &config.Where{
						MethodName: "main.test_array_of_structs",
					},
				},
			},
			expectedDecodingOutput: `{}`,
			expectedToPass:         false,
		},

		// Slice tests
		{
			name:   "test_uint_slice",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_uint_slice",
					Where: &config.Where{
						MethodName: "main.test_uint_slice",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_empty_slice",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_empty_slice",
					Where: &config.Where{
						MethodName: "main.test_empty_slice",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_slice_of_slices",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_slice_of_slices",
					Where: &config.Where{
						MethodName: "main.test_slice_of_slices",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_struct_slice",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_struct_slice",
					Where: &config.Where{
						MethodName: "main.test_struct_slice",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_empty_slice_of_structs",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_empty_slice_of_structs",
					Where: &config.Where{
						MethodName: "main.test_empty_slice_of_structs",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_nil_slice_of_structs",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_nil_slice_of_structs",
					Where: &config.Where{
						MethodName: "main.test_nil_slice_of_structs",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_string_slice",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_string_slice",
					Where: &config.Where{
						MethodName: "main.test_string_slice",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_nil_slice_with_other_params",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_nil_slice_with_other_params",
					Where: &config.Where{
						MethodName: "main.test_nil_slice_with_other_params",
					},
				},
			},
			expectedDecodingOutput: `{"a":1,"s":null,"x":5}`,
			expectedToPass:         true,
		},
		{
			name:   "test_nil_slice",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_nil_slice",
					Where: &config.Where{
						MethodName: "main.test_nil_slice",
					},
				},
			},
			expectedDecodingOutput: `{}`,
			expectedToPass:         false,
		},

		// Struct tests
		{
			name:   "test_struct",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_struct",
					Where: &config.Where{
						MethodName: "main.test_struct",
					},
				},
			},
			expectedDecodingOutput: `{"x":{"aBool":true,"aString":"one","aNumber":2,"nested":{"anotherInt":3,"anotherString":"four"}}}`,
			expectedToPass:         true,
		},
		{
			name:   "test_empty_struct",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_empty_struct",
					Where: &config.Where{
						MethodName: "main.test_empty_struct",
					},
				},
			},
			expectedDecodingOutput: `{}`,
			expectedToPass:         false,
		},

		// Pointer tests
		{
			name:   "test_uint_pointer",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_uint_pointer",
					Where: &config.Where{
						MethodName: "main.test_uint_pointer",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_string_pointer",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_string_pointer",
					Where: &config.Where{
						MethodName: "main.test_string_pointer",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_nil_pointer",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_nil_pointer",
					Where: &config.Where{
						MethodName: "main.test_nil_pointer",
					},
				},
			},
			expectedDecodingOutput: `{}`,
			expectedToPass:         false,
		},

		// Multi-parameter tests
		{
			name:   "test_combined_byte",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_combined_byte",
					Where: &config.Where{
						MethodName: "main.test_combined_byte",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_multiple_simple_params",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_multiple_simple_params",
					Where: &config.Where{
						MethodName: "main.test_multiple_simple_params",
					},
				},
			},
			expectedDecodingOutput: `{}`,
			expectedToPass:         false,
		},

		// Map tests
		{
			name:   "test_map_string_to_int",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_map_string_to_int",
					Where: &config.Where{
						MethodName: "main.test_map_string_to_int",
					},
				},
			},
			expectedDecodingOutput: `{}`,
			expectedToPass:         false,
		},

		// Interface tests
		{
			name:   "test_interface",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_interface",
					Where: &config.Where{
						MethodName: "main.test_interface",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "test_error",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_error",
					Where: &config.Where{
						MethodName: "main.test_error",
					},
				},
			},
			expectedDecodingOutput: `{}`,
			expectedToPass:         false,
		},

		// Complex tests
		{
			name:   "test_big_struct",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_big_struct",
					Where: &config.Where{
						MethodName: "main.test_big_struct",
					},
				},
			},
			expectedDecodingOutput: `{}`,
			expectedToPass:         false,
		},

		// Stack trace tests
		{
			name:   "stack_A",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "stack_A",
					Where: &config.Where{
						MethodName: "main.stack_A",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "stack_B",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "stack_B",
					Where: &config.Where{
						MethodName: "main.stack_B",
					},
				},
			},
			expectedDecodingOutput: ``,
			expectedToPass:         false,
		},
		{
			name:   "stack_C",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "stack_C",
					Where: &config.Where{
						MethodName: "main.stack_C",
					},
				},
			},
			expectedDecodingOutput: `{}`,
			expectedToPass:         false,
		},

		// Other tests
		{
			name:   "test_channel",
			binary: "sample",
			probes: []config.Probe{
				&config.LogProbe{
					ID: "test_channel",
					Where: &config.Where{
						MethodName: "main.test_channel",
					},
				},
			},
			expectedDecodingOutput: `{}`,
			expectedToPass:         false,
		},
	}

	allTestData = make(map[string]TestData)

	for _, tc := range testCases {
		for _, cfg := range testprogs.GetCommonConfigs(t) {
			if cfg.GOARCH != runtime.GOARCH {
				continue
			}

			testName := fmt.Sprintf("%s_%s", tc.name, cfg.String())
			t.Run(testName, func(t *testing.T) {
				bin := testprogs.GetBinary(t, tc.binary, cfg)
				testDyninst(t, testName, bin, tc)
			})
		}
	}
}

var allTestData map[string]TestData

func testDyninst(t *testing.T, testName string, sampleServicePath string, tc testCase) {
	tempDir, err := os.MkdirTemp(os.TempDir(), "dyninst-integration-test-")
	require.NoError(t, err)
	defer func() {
		preserve, _ := strconv.ParseBool(os.Getenv("KEEP_TEMP"))
		if preserve || t.Failed() {
			t.Logf("leaving temp dir %s for inspection", tempDir)
		} else {
			require.NoError(t, os.RemoveAll(tempDir))
		}
	}()

	// Load the binary and generate the IR.
	binary, err := safeelf.Open(sampleServicePath)
	require.NoError(t, err)
	defer func() { require.NoError(t, binary.Close()) }()

	obj, err := object.NewElfObject(binary)
	require.NoError(t, err)
	require.NotNil(t, obj)

	irp, err := irgen.GenerateIR(1, obj, tc.probes)
	require.NoError(t, err)
	require.NotNil(t, irp)
	irDump, err := os.Create(filepath.Join(tempDir, "probe.ir.yaml"))
	require.NoError(t, err)
	require.NotNil(t, irDump)
	defer func() { require.NoError(t, irDump.Close()) }()
	irYaml, err := irprinter.PrintYAML(irp)
	require.NoError(t, err)
	_, err = irDump.Write(irYaml)
	require.NoError(t, err)

	// Compile the IR and prepare the BPF program.
	codeDump, err := os.Create(filepath.Join(tempDir, "probe.bpf.c"))
	require.NoError(t, err)
	require.NotNil(t, codeDump)
	defer func() { require.NoError(t, codeDump.Close()) }()

	compiledBPF, err := compiler.CompileBPFProgram(irp, codeDump)
	require.NoError(t, err)

	bpfObjDump, err := os.Create(filepath.Join(tempDir, "probe.bpf.o"))
	require.NoError(t, err)
	require.NotNil(t, bpfObjDump)
	defer func() { require.NoError(t, bpfObjDump.Close()) }()
	_, err = io.Copy(bpfObjDump, compiledBPF.Obj)
	require.NoError(t, err)

	spec, err := ebpf.LoadCollectionSpecFromReader(compiledBPF.Obj)
	require.NoError(t, err)
	require.NotNil(t, spec)

	bpfCollection, err := ebpf.NewCollectionWithOptions(spec, ebpf.CollectionOptions{})
	require.NoError(t, err)
	require.NotNil(t, bpfCollection)
	defer func() { bpfCollection.Close() }()

	bpfProg, ok := bpfCollection.Programs[compiledBPF.ProgramName]
	require.True(t, ok)
	require.NotNil(t, bpfProg)

	sampleLink, err := link.OpenExecutable(sampleServicePath)
	require.NoError(t, err)
	require.NotNil(t, sampleLink)

	// Extract the method name from the first probe for uprobe attachment
	require.Greater(t, len(tc.probes), 0, "at least one probe must be provided")
	var methodName string
	switch probe := tc.probes[0].(type) {
	case *config.LogProbe:
		methodName = probe.Where.MethodName
	default:
		t.Fatalf("unsupported probe type: %T", probe)
	}

	// Launch the sample service, inject the BPF program and collect the output.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)

	sampleProc := exec.CommandContext(ctx, sampleServicePath)
	require.NotNil(t, sampleProc)
	sampleStdin, err := sampleProc.StdinPipe()
	require.NoError(t, err)
	sampleProc.Stdout, err = os.Create(filepath.Join(tempDir, "sample.out"))
	require.NoError(t, err)
	sampleProc.Stderr, err = os.Create(filepath.Join(tempDir, "sample.err"))
	require.NoError(t, err)
	require.NotNil(t, sampleStdin)
	err = sampleProc.Start()
	require.NoError(t, err)
	attached, err := sampleLink.Uprobe(methodName, bpfProg, &link.UprobeOptions{
		PID:    sampleProc.Process.Pid,
		Cookie: 0,
	})
	require.NoError(t, err)
	require.NotNil(t, attached)
	defer func() { require.NoError(t, attached.Close()) }()

	// Communicate with the sample service to trigger the function calls (ask politely).
	sampleStdin.Write([]byte("Hey! If you don't mind, I'd very much like to trigger a function call. I really appreciate it!\n"))

	err = sampleProc.Wait()
	require.NoError(t, err)

	// Validate the output. For now we just check the total length.
	rd, err := ringbuf.NewReader(bpfCollection.Maps["out_ringbuf"])
	require.NoError(t, err)
	require.NotNil(t, rd)

	bpfOutDump, err := os.Create(filepath.Join(tempDir, "probe.bpf.out"))
	require.NoError(t, err)
	require.NotNil(t, bpfOutDump)
	defer func() { require.NoError(t, bpfOutDump.Close()) }()

	require.Greater(t, rd.AvailableBytes(), 0)
	record, err := rd.Read()
	require.NoError(t, err)
	require.NotNil(t, record)
	bpfOutDump.Write(record.RawSample)

	funcName := strings.ReplaceAll(tc.probes[0].(*config.LogProbe).Where.MethodName, ".", "_")
	allTestData[funcName] = TestData{
		Prog: irp,
		B:    record.RawSample,
	}

	header := (*output.EventHeader)(unsafe.Pointer(&record.RawSample[0]))
	pos := uint32(unsafe.Sizeof(*header)) + uint32(header.Stack_byte_len)
	di := (*output.DataItemHeader)(unsafe.Pointer(&record.RawSample[pos]))
	typ, ok := irp.Types[ir.TypeID(di.Type)]
	require.True(t, ok)
	require.IsType(t, &ir.EventRootType{}, typ)
	require.Equal(t, di.Length, typ.GetByteSize())
	cancel()

	// Decode the data with the corresponding IR used to generate it
	b := []byte{}
	out := bytes.NewBuffer(b)
	decoder, err := decode.NewDecoder(irp)
	require.NoError(t, err)
	require.NotNil(t, decoder)

	err = decoder.Decode(record.RawSample, out)
	require.NoError(t, err)
	require.Equal(t, tc.expectedDecodingOutput, strings.TrimSpace(out.String()))
	fmt.Println("Output:", out.String())
}

type TestData struct {
	Prog *ir.Program
	B    []byte
}
