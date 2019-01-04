// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package collector

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/stretchr/testify/assert"
)

type MockLoader struct{}

func (l *MockLoader) Load(config integration.Config) ([]check.Check, error) {
	return []check.Check{}, nil
}

func TestAddLoader(t *testing.T) {
	s := CheckScheduler{}
	assert.Len(t, s.loaders, 0)
	s.AddLoader(&MockLoader{})
	s.AddLoader(&MockLoader{}) // noop
	assert.Len(t, s.loaders, 1)
}
