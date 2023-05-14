// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
