// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package filters

import (
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"

	"github.com/stretchr/testify/assert"
)

func TestBlacklister(t *testing.T) {
	tests := []struct {
		filter      []string
		resource    string
		expectation bool
	}{
		{[]string{"/foo/bar"}, "/foo/bar", false},
		{[]string{"/foo/b.r"}, "/foo/bar", false},
		{[]string{"[0-9]+"}, "/abcde", true},
		{[]string{"[0-9]+"}, "/abcde123", false},
		{[]string{"\\(foobar\\)"}, "(foobar)", false},
		{[]string{"\\(foobar\\)"}, "(bar)", true},
		{[]string{"(GET|POST) /healthcheck"}, "GET /foobar", true},
		{[]string{"(GET|POST) /healthcheck"}, "GET /healthcheck", false},
		{[]string{"(GET|POST) /healthcheck"}, "POST /healthcheck", false},
		{[]string{"SELECT COUNT\\(\\*\\) FROM BAR"}, "SELECT COUNT(*) FROM BAR", false},
		{[]string{"[123"}, "[123", true},
		{[]string{"\\[123"}, "[123", false},
		{[]string{"ABC+", "W+"}, "ABCCCC", false},
		{[]string{"ABC+", "W+"}, "WWW", false},
	}

	for _, test := range tests {
		span := testutil.RandomSpan()
		stat := pb.ClientGroupedStats{Resource: test.resource}
		span.Resource = test.resource
		filter := NewBlacklister(test.filter)

		assert.Equal(t, test.expectation, filter.Allows(span))
		assert.Equal(t, test.expectation, filter.AllowsStat(&stat))
	}
}

func TestCompileRules(t *testing.T) {
	filter := NewBlacklister([]string{"[123", "]123", "{6}"})
	for i := 0; i < 100; i++ {
		span := testutil.RandomSpan()
		stat := pb.ClientGroupedStats{Resource: span.Resource}
		assert.True(t, filter.Allows(span))
		assert.True(t, filter.AllowsStat(&stat))
	}
}
