// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/stretchr/testify/assert"
)

func TestNewScheduler(t *testing.T) {
	fwd := forwarder.NewDefaultForwarder(nil)
	fwd.Start()
	s := serializer.NewSerializer(fwd)
	c := NewScheduler(s, "hostname")
	assert.Equal(t, fwd, c.srl.Forwarder)
	assert.Equal(t, "hostname", c.hostname)
}

func TestStopScheduler(t *testing.T) {
	fwd := forwarder.NewDefaultForwarder(nil)
	fwd.Start()
	s := serializer.NewSerializer(fwd)
	c := NewScheduler(s, "hostname")
	c.AddCollector("test", time.Duration(60))
	c.AddCollector("test2", time.Duration(60))
	c.Stop()
	assert.Equal(t, context.Canceled, c.context.Err())
}
