// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ckey_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unique"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// copy of toUnique because "github.com/DataDog/datadog-agent" is not an allowed import for "/go/src/github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
func toUnique(ss []string) []unique.Handle[string] {
	if ss == nil {
		return nil
	}
	ret := make([]unique.Handle[string], 0, len(ss))
	for _, s := range ss {
		ret = append(ret, unique.Make(s))
	}
	return ret
}

func TestCollisions(t *testing.T) {
	assert := assert.New(t)

	data, err := os.ReadFile("./random_sorted_uniq_contexts.csv")
	assert.NoError(err)

	generator := ckey.NewKeyGenerator()
	host := "host"

	var cache = make(map[ckey.ContextKey]string)
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		parts := strings.Split(line, ",")
		if i == len(lines)-1 {
			break // last line
		}
		assert.Len(parts, 2, "Format is: metric_name,tag1 tag2 tag3")
		metricName := parts[0]
		tagList := parts[1]
		tags := toUnique(strings.Split(tagList, " "))
		ck := generator.Generate(metricName, host, tagset.NewHashingTagsAccumulatorWithTags(tags))
		if v, exists := cache[ck]; exists {
			assert.Fail("A collision happened:", v, "and", line)
		} else {
			cache[ck] = line
		}
	}

	fmt.Println("Tested", len(cache), "contexts, no collision found")
}
