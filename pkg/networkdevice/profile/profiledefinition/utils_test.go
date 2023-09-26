package profiledefinition

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestKeyValueList_ToMap(t *testing.T) {
	kvList := KeyValueList{
		{Key: "1", Value: "aaa"},
		{Key: "2", Value: "bbb"},
	}
	assert.Equal(t, map[string]string{"1": "aaa", "2": "bbb"}, kvList.ToMap())

	emptyKvList := KeyValueList{}
	assert.Equal(t, map[string]string{}, emptyKvList.ToMap())
}
