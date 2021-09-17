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

func Test_resultValueStore_GetScalarValueAsString(t *testing.T) {
	assert.Equal(t, "", storeMock.GetScalarValueAsString("0.0"))
	assert.Equal(t, "10", storeMock.GetScalarValueAsString("1.1.1.1.0"))
	assert.Equal(t, "a_str_value", storeMock.GetScalarValueAsString("1.1.1.2.0"))
	assert.Equal(t, "", storeMock.GetScalarValueAsString("1.1.1.3.0"))
}

func Test_resultValueStore_getColumnValueAsString(t *testing.T) {
	assert.Equal(t, "", storeMock.GetColumnValueAsString("0.0", "1"))              // wrong column
	assert.Equal(t, "10", storeMock.GetColumnValueAsString("1.1.1", "1"))          // ok float value
	assert.Equal(t, "a_str_value", storeMock.GetColumnValueAsString("1.1.1", "2")) // ok str value
	assert.Equal(t, "", storeMock.GetColumnValueAsString("1.1.1", "3"))            // wrong type
	assert.Equal(t, "", storeMock.GetColumnValueAsString("1.1.1", "99"))           // index not found
	assert.Equal(t, "21", storeMock.GetColumnValueAsString("1.1.2", "1"))          // ok float value
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
