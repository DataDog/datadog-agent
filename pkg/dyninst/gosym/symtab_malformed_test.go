// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package gosym_test

import (
	"encoding/binary"
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
