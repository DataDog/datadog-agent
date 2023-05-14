// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/common"
)

func contains(want string, strings []string) bool {
	for _, candidate := range strings {
		if want == candidate {
			return true
		}
	}
	return false
}

// Example: given {"_opt3", "tag:a", "_opt2", "tag:b", "_opt1", "tag:c"}
// The return value will be:
// {"tag:a", "tag:b", "tag:c"}
// {"_opt1", "_opt2", "_opt3"}
func splitTagsAndOptions(all []string) (tags, opts []string) {
	if len(all) == 0 {
		return
	}
	tagSet := common.NewStringSet()
	optSet := common.NewStringSet()

	for _, s := range all {
		if strings.HasPrefix(s, optPrefix) {
			optSet.Add(s)
		} else {
			tagSet.Add(s)
		}
	}

	tags = tagSet.GetAll()
	opts = optSet.GetAll()

	// we sort both tags and options so the order is always deterministic and
	// comparison between different tag sets is more efficient (see `isEqual`)
	sort.Strings(tags)
	sort.Strings(opts)

	return
}

// insertNestedValueFor is used for translating a set of "flat" metric names into
// a nested map representation.
// Usage Example:
// metrics := make(map[string]interface{})
// insertNestedValueFor("http.request_count", 1, metrics)
// insertNestedValueFor("dns.errors.nxdomain", 5, metrics)
// Results in:
//
//	{
//	  "http": {
//	    "request_count": 1
//	  },
//	  "dns": {
//	    "errors": {
//	      "nxdomain": 5
//	    }
//	  }
//	}
func insertNestedValueFor(name string, value int64, root map[string]interface{}) error {
	parts := strings.Split(name, ".")
	if len(parts) == 1 {
		root[name] = value
		return nil
	}

	parent := root
	for i := 0; i < len(parts)-1; i++ {
		if v, ok := parent[parts[i]]; ok {
			child, ok := v.(map[string]interface{})

			if !ok {
				return fmt.Errorf("invalid value type (%T)", v)
			}

			parent = child
			continue
		}

		child := make(map[string]interface{})
		parent[parts[i]] = child
		parent = child
	}

	parent[parts[len(parts)-1]] = value
	return nil
}
