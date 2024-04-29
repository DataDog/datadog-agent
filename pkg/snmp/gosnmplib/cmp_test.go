// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gosnmplib_test

import (
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOIDToInts(t *testing.T) {
	for _, tc := range []struct {
		oid      string
		expected []int
	}{
		{"1.2.3.4", []int{1, 2, 3, 4}},
		{".0.101.99999.1.1.1.3.0.0.", []int{0, 101, 99999, 1, 1, 1, 3, 0, 0}},
		{"0", []int{0}},
		{"..0..", []int{0}},
		{".", nil},
		{"", nil},
		{".-1.-10.10.-1", []int{-1, -10, 10, -1}}, // this shouldn't come up but better safe than sorry
	} {
		t.Run(tc.oid, func(t *testing.T) {
			got, err := gosnmplib.OIDToInts(tc.oid)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}

	t.Run("Failure", func(t *testing.T) {
		_, err := gosnmplib.OIDToInts("non-numeric.1.2.3")
		var e *strconv.NumError
		assert.ErrorAs(t, err, &e)
	})
}

func TestOIDRelation(t *testing.T) {
	for _, tc := range []struct {
		val    gosnmplib.OIDRelation
		after  bool
		before bool
	}{
		{gosnmplib.EQUAL, false, false},
		{gosnmplib.CHILD, true, false},
		{gosnmplib.PARENT, false, true},
		{gosnmplib.GREATER, true, false},
		{gosnmplib.LESS, false, true},
	} {
		assert.Equal(t, tc.after, tc.val.IsAfter())
		assert.Equal(t, tc.before, tc.val.IsBefore())
	}
}

func TestCmpOIDs(t *testing.T) {
	for _, tc := range []struct {
		a, b   []int
		result gosnmplib.OIDRelation
	}{
		{[]int{1, 2, 3}, []int{1, 2, 3}, gosnmplib.EQUAL},
		{[]int{1, 2, 3}, []int{1, 2}, gosnmplib.CHILD},
		{[]int{1, 2}, []int{1, 2, 3}, gosnmplib.PARENT},
		{[]int{1, 2, 3}, []int{1, 2, 4}, gosnmplib.LESS},
		{[]int{1, 2, 3}, []int{1, 2, 2}, gosnmplib.GREATER},
		{[]int{1, 1, 2, 3, 5, 8, 13}, []int{1, 2, 1, 2, 4, 7, 12}, gosnmplib.LESS},
		{[]int{1, 1}, []int{1, 2, 1, 2, 4, 7, 12}, gosnmplib.LESS},
		{[]int{1, 10}, []int{1, 2, 1, 2, 4, 7, 12}, gosnmplib.GREATER},
		{[]int{1, 2}, []int{1, 2, 1, 2, 4, 7, 12}, gosnmplib.PARENT},
	} {
		assert.Equal(t, tc.result, gosnmplib.CmpOIDs(tc.a, tc.b))
	}
}
