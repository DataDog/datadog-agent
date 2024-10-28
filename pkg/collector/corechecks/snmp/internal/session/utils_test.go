// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNextColumnOid(t *testing.T) {
	tests := []struct {
		oid         string
		expectedOid string
		expectedErr string
	}{
		{
			oid:         "1.3.6.1.2.1.2.2.1.2.99",
			expectedOid: "1.3.6.1.2.1.2.2.1.3",
		},
		{
			oid:         "1.3.6.1.2.1.2.2.1.99.99",
			expectedOid: "1.3.6.1.2.1.2.2.1.100",
		},
		{
			oid: "1.3.6.1.2.1.2.2.1.1.1",
			// ideally it should return "1.3.6.1.2.1.2.2.1.2" instead of `1.3.6.1.2.1.2.2.1.1.2`
			// but we can improve the algorithm later if needed
			expectedOid: "1.3.6.1.2.1.2.2.1.1.2",
		},
		{
			oid:         "1.3.6.2.2.2.2.2.99999",
			expectedErr: "the oid is not a column oid: 1.3.6.2.2.2.2.2.99999",
		},
		{
			oid:         ".1.3.6.1.2.1.2.2.1.2.99.", // trim trailing/ending `.`
			expectedOid: "1.3.6.1.2.1.2.2.1.3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.oid, func(t *testing.T) {
			newOid, err := GetNextColumnOidNaive(tt.oid)
			assert.Equal(t, tt.expectedOid, newOid)

			if tt.expectedErr != "" {
				assert.EqualError(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
