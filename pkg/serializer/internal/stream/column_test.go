// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stream

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	zlib "github.com/DataDog/datadog-agent/pkg/util/compression/impl-zlib"
)

func TestColumn(t *testing.T) {
	cs := zlib.New()
	cc := NewColumnCompressor(cs, 10, 67, 128)

	txn := cc.NewTransaction()

	txn.Uint64(1, 0xffff)
	txn.Uint64(2, 0xffff)
	txn.Uint64(3, 0xffff)

	assert.Equal(t, []byte{255, 255, 3}, txn.inputs[1])
	assert.Equal(t, []byte{255, 255, 3}, txn.inputs[2])
	assert.Equal(t, []byte{255, 255, 3}, txn.inputs[3])
	assert.Equal(t, 9, txn.length)

	err := cc.AddItem(txn)
	assert.NoError(t, err)

	assert.Equal(t, 9, cc.inputLength)
	assert.Equal(t, 9, cc.totalLength)
	assert.Equal(t, 0, cc.outputLength)

	err = cc.AddItem(txn)
	assert.NoError(t, err)

	assert.Equal(t, 18, cc.inputLength)
	assert.Equal(t, 18, cc.totalLength)
	assert.Equal(t, 0, cc.outputLength)

	err = cc.AddItem(txn)
	assert.NoError(t, err)

	assert.Equal(t, 27, cc.inputLength)
	assert.Equal(t, 27, cc.totalLength)
	assert.Equal(t, 0, cc.outputLength)

	err = cc.AddItem(txn)
	assert.NoError(t, err)

	assert.Equal(t, 36, cc.inputLength)
	assert.Equal(t, 36, cc.totalLength)
	assert.Equal(t, 0, cc.outputLength)

	err = cc.AddItem(txn)
	assert.NoError(t, err)

	assert.Equal(t, 45, cc.inputLength)
	assert.Equal(t, 45, cc.totalLength)
	assert.Equal(t, 0, cc.outputLength)

	err = cc.AddItem(txn)
	assert.NoError(t, err)

	assert.Equal(t, 9, cc.inputLength)
	assert.Equal(t, 54, cc.totalLength)
	assert.Equal(t, 39, cc.outputLength)

	txn.Reset()
	for i := 0; i < 10; i++ {
		txn.Uint64(i, ^uint64(0))
	}

	err = cc.AddItem(txn)
	assert.Equal(t, err, ErrPayloadFull)

	err = cc.Close()
	assert.NoError(t, err)

	var combined []byte
	for i := 0; i < len(cc.columns); i++ {
		if cc.UncompressedLen(i) > 0 {
			col, err := cs.Decompress(cc.CompressedBytes(i))
			assert.NoError(t, err)
			combined = append(combined, col...)
		}

		if i > 0 && i <= 3 {
			assert.Equal(t, cc.UncompressedLen(i), 18)
		} else {
			assert.Equal(t, cc.UncompressedLen(i), 0)
		}
	}

	assert.Equal(t, slices.Repeat([]byte{255, 255, 3}, 6*3), combined)

	cc.Reset()
	txn.Reset()
	txn.Uint64(1, 1)

	assert.Equal(t, 0, cc.totalLength)

	err = cc.AddItem(txn)
	assert.NoError(t, err)

	cc.Close()

	for i := 0; i < len(cc.columns); i++ {
		if i == 1 {
			assert.Equal(t, cc.UncompressedLen(i), 1)
		} else {
			assert.Equal(t, cc.UncompressedLen(i), 0)
		}
	}
}
