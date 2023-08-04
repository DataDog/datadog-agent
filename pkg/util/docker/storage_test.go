// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"fmt"
	"math"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"
)

func createIntPointer(value float64) *uint64 {
	val := uint64(value)
	return &val
}

func TestParseStorageStats(t *testing.T) {
	assert := assert.New(t)
	for nb, tc := range []struct {
		source   [][2]string
		values   []*StorageStats
		percents map[string]float64
		err      error
	}{
		{
			// Return standardized error if no info, to print an integration warning
			source: [][2]string{},
			values: []*StorageStats{},
			err:    ErrStorageStatsNotAvailable,
		},
		{
			// Nominal case
			source: [][2]string{
				{"Data Space Used", "1 GB"},
				{"Data Space Available", "9 GB"},
				{"Data Space Total", "10 GB"},
				{"Metadata Space Used", "1 MB"},
				{"Metadata Space Available", "9 MB"},
				{"Metadata Space Total", "10 MB"},
			},
			values: []*StorageStats{
				{
					Name:  DataStorageName,
					Free:  createIntPointer(9e9),
					Used:  createIntPointer(1e9),
					Total: createIntPointer(10e9),
				},
				{
					Name:  MetadataStorageName,
					Free:  createIntPointer(9e6),
					Used:  createIntPointer(1e6),
					Total: createIntPointer(10e6),
				},
			},
			percents: map[string]float64{
				DataStorageName:     10,
				MetadataStorageName: 10,
			},
			err: nil,
		},
		{
			// Only metadata, mixed case to test unit lowercasing
			source: [][2]string{
				{"Metadata Space Used", "1 kb"},
				{"Metadata Space Available", "9 KB"},
				{"Metadata Space Total", "10 kB"},
			},
			values: []*StorageStats{
				{
					Name:  MetadataStorageName,
					Free:  createIntPointer(9e3),
					Used:  createIntPointer(1e3),
					Total: createIntPointer(10e3),
				},
			},
			percents: map[string]float64{
				MetadataStorageName: 10,
			},
			err: nil,
		},
		{
			// Only metadata, decimal values
			source: [][2]string{
				{"Metadata Space Used", "0.100 Mb"},
				{"Metadata Space Available", "0.9 MB"},
				{"Metadata Space Total", "1 MB"},
			},
			values: []*StorageStats{
				{
					Name:  MetadataStorageName,
					Free:  createIntPointer(9e5),
					Used:  createIntPointer(1e5),
					Total: createIntPointer(1e6),
				},
			},
			percents: map[string]float64{
				MetadataStorageName: 10,
			},
			err: nil,
		},
		{
			// Missing one line: percentages should still be computed
			source: [][2]string{
				{"NoUsed Space Available", "9 GB"},
				{"NoUsed Space Total", "10 GB"},
				{"NoFree Space Used", "9 MB"},
				{"NoFree Space Total", "10 MB"},
				{"NoTotal Space Available", "2 MB"},
				{"NoTotal Space Used", "8 MB"},
			},
			values: []*StorageStats{
				{
					Name:  "noused",
					Free:  createIntPointer(9e9),
					Used:  nil,
					Total: createIntPointer(10e9),
				},
				{
					Name:  "nofree",
					Free:  nil,
					Used:  createIntPointer(9e6),
					Total: createIntPointer(10e6),
				},
				{
					Name:  "nototal",
					Free:  createIntPointer(2e6),
					Used:  createIntPointer(8e6),
					Total: nil,
				},
			},
			percents: map[string]float64{
				"noused":  10,
				"nofree":  90,
				"nototal": 80,
			},
			err: nil,
		},
		{
			// All zeroes
			source: [][2]string{
				{"Data Space Available", "0 GB"},
				{"Data Space Total", "0 GB"},
				{"Data Space Used", "0 GB"},
			},
			values: []*StorageStats{
				{
					Name:  DataStorageName,
					Free:  createIntPointer(0.0),
					Used:  createIntPointer(0.0),
					Total: createIntPointer(0.0),
				},
			},
			percents: map[string]float64{
				DataStorageName: math.NaN(),
			},
			err: nil,
		},
		{
			// Invalid total, percent will be computed from used/used+free
			source: [][2]string{
				{"Data Space Available", "9 GB"},
				{"Data Space Total", "10 GB"},
				{"Data Space Used", "11 GB"},
			},
			values: []*StorageStats{
				{
					Name:  DataStorageName,
					Free:  createIntPointer(9e9),
					Used:  createIntPointer(11e9),
					Total: createIntPointer(10e9),
				},
			},
			percents: map[string]float64{
				DataStorageName: 55,
			},
			err: nil,
		},
		{
			// Only metadata, no spaces before units
			source: [][2]string{
				{"Metadata Space Used", "1kb"},
				{"Metadata Space Available", "19KB"},
				{"Metadata Space Total", "20kB"},
			},
			values: []*StorageStats{
				{
					Name:  MetadataStorageName,
					Free:  createIntPointer(19e3),
					Used:  createIntPointer(1e3),
					Total: createIntPointer(20e3),
				},
			},
			percents: map[string]float64{
				MetadataStorageName: 5,
			},
			err: nil,
		},
		{
			// Invalid formats
			source: [][2]string{
				{"Metadata Space Used", "1"},
				{"Metadata pace Available", "19KB"},
				{"Metadata Total", "20kB"},
			},
			values:   []*StorageStats{},
			percents: map[string]float64{},
			err:      nil,
		},
	} {
		t.Logf("test case %d", nb)
		info := types.Info{
			DriverStatus: tc.source,
		}
		stat, err := parseStorageStatsFromInfo(info)
		assert.Equal(tc.err, err)
		assert.Equal(tc.values, stat)
		for _, val := range stat {
			expected := tc.percents[val.Name]
			if math.IsNaN(expected) {
				assert.True(math.IsNaN(val.GetPercentUsed()))
			} else {
				assert.Equal(tc.percents[val.Name], val.GetPercentUsed())
			}
		}
	}
}

func TestParseDiskQuantity(t *testing.T) {
	assert := assert.New(t)
	for nb, tc := range []struct {
		text  string
		bytes uint64
		err   error
	}{
		// Nominal cases
		{"10 b", 10, nil},
		{"521kb", 521000, nil},
		{"0 MB", 0, nil},
		// Unknown unit
		{"10 AB", 0, fmt.Errorf("parsing error: unknown unit AB")},
		// Parsing error
		{"10", 0, fmt.Errorf("parsing error: invalid format")},
		{"MB 10", 0, fmt.Errorf("parsing error: invalid format")},
	} {
		t.Logf("test case %d", nb)
		val, err := parseDiskQuantity(tc.text)
		assert.Equal(tc.bytes, val)

		if tc.err == nil {
			assert.Nil(err)
		} else {
			assert.NotNil(err)
			assert.Equal(tc.err.Error(), err.Error())
		}
	}
}
