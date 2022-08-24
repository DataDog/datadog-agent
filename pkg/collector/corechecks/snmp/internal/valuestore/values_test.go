// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package valuestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var storeMock = &ResultValueStore{
	ScalarValues: ScalarResultValuesType{
		"1.1.1.1.0": {Value: float64(10)},   // a float value
		"1.1.1.2.0": {Value: "a_str_value"}, // a string value
		"1.1.1.3.0": {Value: nil},           // invalid type value
	},
	ColumnValues: ColumnResultValuesType{
		"1.1.1": {
			"1": ResultValue{Value: float64(10)},   // a float value
			"2": ResultValue{Value: "a_str_value"}, // a string value
			"3": ResultValue{Value: nil},           // invalid type value
		},
		"1.1.2": {
			"1": ResultValue{Value: float64(21)},
			"2": ResultValue{Value: float64(22)},
		},
	},
}

func Test_resultValueStore_getColumnValueAsFloat(t *testing.T) {
	assert.Equal(t, float64(0), storeMock.GetColumnValueAsFloat("0.0", "1"))    // wrong column
	assert.Equal(t, float64(10), storeMock.GetColumnValueAsFloat("1.1.1", "1")) // ok float value
	assert.Equal(t, float64(0), storeMock.GetColumnValueAsFloat("1.1.1", "2"))  // cannot convert str to float
	assert.Equal(t, float64(0), storeMock.GetColumnValueAsFloat("1.1.1", "3"))  // wrong type
	assert.Equal(t, float64(0), storeMock.GetColumnValueAsFloat("1.1.1", "99")) // index not found
	assert.Equal(t, float64(21), storeMock.GetColumnValueAsFloat("1.1.2", "1")) // ok float value
}

func Test_resultValueStore_GetColumnIndexes(t *testing.T) {
	indexes, err := storeMock.GetColumnIndexes("0.0")
	assert.EqualError(t, err, "error getting column value oid=0.0: value for Column OID `0.0` not found in results")
	assert.Nil(t, indexes)

	indexes, err = storeMock.GetColumnIndexes("1.1.1")
	assert.NoError(t, err)
	assert.Equal(t, []string{"1", "2", "3"}, indexes)
}

func TestResultValueStoreAsString(t *testing.T) {
	store := &ResultValueStore{
		ScalarValues: ScalarResultValuesType{
			"1.1.1.1.0": {Value: float64(10)}, // a float value
		},
		ColumnValues: ColumnResultValuesType{
			"1.1.1": {
				"1": ResultValue{Value: float64(10)}, // a float value
			},
		},
	}
	str := ResultValueStoreAsString(store)
	assert.Equal(t, "{\"scalar_values\":{\"1.1.1.1.0\":{\"value\":10}},\"column_values\":{\"1.1.1\":{\"1\":{\"value\":10}}}}", str)

	str = ResultValueStoreAsString(nil)
	assert.Equal(t, "", str)

}
