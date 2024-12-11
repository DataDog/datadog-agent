// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

func TestLegacyCases(t *testing.T) {
	assert := assert.New(t)
	e := NewLegacyExtractor(map[string]float64{"serviCE1": 1})
	span := &pb.Span{Service: "SeRvIcE1"}
	traceutil.SetTopLevel(span, true)

	rate, ok := e.Extract(span, 0)
	assert.Equal(rate, 1.)
	assert.True(ok)
}
