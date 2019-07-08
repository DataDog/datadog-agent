package traceutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateString(t *testing.T) {
	assert.Equal(t, "", TruncateUTF8("", 5))
	assert.Equal(t, "télé", TruncateUTF8("télé", 5))
	assert.Equal(t, "t", TruncateUTF8("télé", 2))
	assert.Equal(t, "éé", TruncateUTF8("ééééé", 5))
	assert.Equal(t, "ééééé", TruncateUTF8("ééééé", 18))
	assert.Equal(t, "ééééé", TruncateUTF8("ééééé", 10))
	assert.Equal(t, "ééé", TruncateUTF8("ééééé", 6))
}
