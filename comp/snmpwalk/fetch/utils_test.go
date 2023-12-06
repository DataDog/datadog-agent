// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fetch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNextColumnOid(t *testing.T) {
	tests := []struct {
		name        string
		oid         string
		expectedOid string
		expectedErr string
	}{
		{
			name:        "OK column case",
			oid:         "1.3.6.1.2.1.2.2.1.2.99",
			expectedOid: "1.3.6.1.2.1.2.2.1.3",
		},
		{
			name:        "OK column case 2",
			oid:         "1.3.6.1.2.1.2.2.1.99.99",
			expectedOid: "1.3.6.1.2.1.2.2.1.100",
		},
		{
			name: "ERROR oid is not column",
			oid:  "1.3.6.2.2.2.2.2.99999",
			//expectedErr: "the oid is not a column oid: 1.3.6.2.2.2.2.2.99999",
			expectedOid: "1.3.6.2.2.2.2.2.99999",
		},
		{
			name:        "OK test trim trailing dot",
			oid:         ".1.3.6.1.2.1.2.2.1.2.99.", // trim trailing/ending `.`
			expectedOid: "1.3.6.1.2.1.2.2.1.3",
		},
		{
			name:        "OK with ending .1",
			oid:         "1.3.6.1.2.1.2.2.1.1.1",
			expectedOid: "1.3.6.1.2.1.2.2.1.2",
		},
		{
			name: "ERROR not a column since .1. entry must be followed by COLUMN.INDEX",
			oid:  "1.3.6.2.2.2.2.2.1.9",
			//expectedErr: "the oid is not a column oid: 1.3.6.2.2.2.2.2.1.9",
			expectedOid: "1.3.6.2.2.2.2.2.1.9",
		},
		{
			name:        "OK 1.0 is valid prefix ",
			oid:         "1.0.6.1.1.2.9",
			expectedOid: "1.0.6.1.1.3",
		},
		{
			name:        "OK increment oid before .0.",
			oid:         "1.3.1.2.3.0.6.1.1.2.9",
			expectedOid: "1.3.1.3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newOid := GetNextColumnOidNaive(tt.oid)
			assert.Equal(t, tt.expectedOid, newOid)
			//
			//if tt.expectedErr != "" {
			//	assert.EqualError(t, err, tt.expectedErr)
			//} else {
			//	assert.NoError(t, err)
			//}
		})
	}
}
