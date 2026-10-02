// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package gosym_test

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
)

const (
	go12Magic  = 0xfffffffb
	go116Magic = 0xfffffffa
	go118Magic = 0xfffffff0
	go120Magic = 0xfffffff1
)

func makePclntab(magic uint32, words int) []byte {
	data := make([]byte, 8+words*8+64)
	binary.LittleEndian.PutUint32(data[0:4], magic)
	data[6] = 1 // quantum
	data[7] = 8 // ptrSize
	return data
}

func setWord(data []byte, word int, val uint64) {
	binary.LittleEndian.PutUint64(data[8+word*8:], val)
}

// An offset word that goes negative when converted to a signed int must be
// rejected rather than used as a slice index. See VULN-100740.
func TestParseRejectsOverflowingOffsets(t *testing.T) {
	const poison = uint64(0x8000000000000000)

	// Each version reads a different set of offset words.
	versions := []struct {
		name  string
		magic uint32
		words []int
	}{
		{"go1.2", go12Magic, []int{0}},
		{"go1.16", go116Magic, []int{0, 1, 2, 3, 4, 5, 6}},
		{"go1.18", go118Magic, []int{0, 1, 3, 4, 5, 6, 7}},
		{"go1.20", go120Magic, []int{0, 1, 3, 4, 5, 6, 7}},
	}

	for _, v := range versions {
		t.Run(v.name, func(t *testing.T) {
			for _, word := range v.words {
				t.Run(fmt.Sprintf("word%d", word), func(t *testing.T) {
					data := makePclntab(v.magic, 8)
					setWord(data, word, poison)

					_, err := gosym.ParseGoSymbolTable(data, nil, 0, 0, 0, 0)
					require.Error(t, err)
				})
			}
		})
	}
}

func TestParseRejectsOversizedTables(t *testing.T) {
	for _, magic := range []uint32{go12Magic, go116Magic, go118Magic, go120Magic} {
		t.Run(strconv.FormatUint(uint64(magic), 16), func(t *testing.T) {
			data := makePclntab(magic, 8)
			setWord(data, 0, 0xffffffff)

			_, err := gosym.ParseGoSymbolTable(data, nil, 0, 0, 0, 0)
			require.Error(t, err)
		})
	}
}
