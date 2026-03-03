// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/go-json-experiment/json/jsontext"
)

// iterateHMapBucket iterates through an hmap bucket and calls the callback
// for each valid entry. Handles overflow buckets recursively.
func iterateHMapBucket(
	c *encodingContext,
	h *goHMapHeaderType,
	bucketData []byte,
	maxItems int,
	callback mapEntryCallback,
) (itemsProcessed int, err error) {
	// See https://github.com/golang/go/blob/66d34c7d/src/runtime/map.go#L90-L99
	const (
		emptyRest      = 0 // this cell is empty, and there are no more non-empty cells
		emptyOne       = 1 // this cell is empty
		evacuatedX     = 2 // key/elem is valid.  Entry has been evacuated to first half of larger table.
		evacuatedEmpty = 4 // cell is empty, bucket is evacuated.
		topHashSize    = 8
	)
	upperBound := max(
		h.keysOffset+h.keyTypeSize*topHashSize,
		h.valuesOffset+h.valueTypeSize*topHashSize,
		h.tophashOfset+topHashSize,
		h.overflowOffset+8,
	)
	if upperBound > uint32(len(bucketData)) {
		return itemsProcessed, fmt.Errorf(
			"hmap bucket data for %q is too short to contain all fields: %d > %d",
			h.Name, upperBound, len(bucketData),
		)
	}
	topHash := bucketData[h.tophashOfset : h.tophashOfset+topHashSize]
	for i, b := range topHash {
		if shouldStop(maxItems, itemsProcessed) {
			break
		}
		if b == emptyRest || (b >= evacuatedX && b <= evacuatedEmpty) {
			break
		}
		if b == emptyOne {
			continue
		}
		keyOffset := h.keysOffset + uint32(i)*h.keyTypeSize
		valueOffset := h.valuesOffset + uint32(i)*h.valueTypeSize
		keyData := bucketData[keyOffset : keyOffset+h.keyTypeSize]
		valueData := bucketData[valueOffset : valueOffset+h.valueTypeSize]

		itemsProcessed++
		shouldContinue, err := callback(keyData, valueData, i)
		if err != nil {
			return itemsProcessed, err
		}
		if !shouldContinue {
			return itemsProcessed, nil
		}
	}
	overflowAddr := binary.NativeEndian.Uint64(bucketData[h.overflowOffset : h.overflowOffset+8])
	if overflowAddr != 0 {
		overflowDataItem, ok := c.getPtr(overflowAddr, h.bucketTypeID)
		var overflowData []byte
		if ok {
			overflowData, ok = overflowDataItem.Data()
		}
		if ok {
			// Preserve unlimitedItems if it was unlimited, otherwise ensure we
			// don't pass negative values.
			remainingItems := maxItems
			if maxItems != unlimitedItems {
				remainingItems = maxItems - itemsProcessed
				if remainingItems < 0 {
					remainingItems = 0
				}
			}
			overflowItems, err := iterateHMapBucket(
				c, h, overflowData, remainingItems, callback,
			)
			if err != nil {
				return itemsProcessed, err
			}
			itemsProcessed += overflowItems
		}
	}
	return itemsProcessed, nil
}

// formatHMapBucket formats entries from a single hmap bucket.
func formatHMapBucket(
	c *encodingContext,
	buf *bytes.Buffer,
	h *goHMapHeaderType,
	bucketData []byte,
	needComma bool,
	limits *formatLimits,
) (formattedItems int, err error) {
	maxItems := limits.maxCollectionItems
	callback := makeFormatMapEntryCallback(
		c, buf, limits, needComma,
		h.BucketType.KeyType,
		h.BucketType.ValueType,
	)
	formattedItems, err = iterateHMapBucket(c, h, bucketData, maxItems, callback)
	return formattedItems, err
}

// encodeHMapBucket encodes entries from a single hmap bucket.
func encodeHMapBucket(
	c *encodingContext,
	enc *jsontext.Encoder,
	h *goHMapHeaderType,
	bucketData []byte,
) (encodedItems int, err error) {
	callback := makeEncodeMapEntryCallback(
		c, enc,
		h.keyTypeID, h.keyTypeName,
		h.valueTypeID, h.valueTypeName,
	)
	return iterateHMapBucket(c, h, bucketData, unlimitedItems, callback)
}
