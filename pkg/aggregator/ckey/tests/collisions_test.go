package ckey_test

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestCollisions(t *testing.T) {
	assert := assert.New(t)

	data, err := ioutil.ReadFile("./random_sorted_uniq_contexts.csv")
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
		tags := strings.Split(tagList, " ")
		ck := generator.Generate(metricName, host, util.NewTagsBuilderFromSlice(tags))
		if v, exists := cache[ck]; exists {
			assert.Fail("A collision happened:", v, "and", line)
		} else {
			cache[ck] = line
		}
	}

	fmt.Println("Tested", len(cache), "contexts, no collision found")
}
