// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func countViperNames(options []*Option, namesMap map[string]int) {
	for _, o := range options {
		namesMap[o.viperName] = namesMap[o.viperName] + 1

		if len(o.SubOptions) > 0 {
			countViperNames(o.SubOptions, namesMap)
		}
	}
}

func TestNoDupeOptionName(t *testing.T) {
	namesMap := make(map[string]int)
	for _, g := range configGroups {
		countViperNames(g.Options, namesMap)
	}

	for name, count := range namesMap {
		assert.Equal(t, 1, count, fmt.Sprintf("Option %s is found %d times", name, count))
	}
}
