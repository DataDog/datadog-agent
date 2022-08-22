package telemetry

import (
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
	sort.Strings(tags)
	sort.Strings(opts)

	return
}

func insertNestedValueFor(name string, value int64, root map[string]interface{}) {
	parts := strings.Split(name, ".")
	if len(parts) == 1 {
		root[name] = value
		return
	}

	parent := root
	for i := 0; i < len(parts)-1; i++ {
		if v, ok := parent[parts[i]]; ok {
			child, ok := v.(map[string]interface{})

			if !ok {
				// shouldn't happen; bail out.
				return
			}

			parent = child
			continue
		}

		child := make(map[string]interface{})
		parent[parts[i]] = child
		parent = child
	}

	parent[parts[len(parts)-1]] = value
}
