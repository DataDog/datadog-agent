package uuid

import "github.com/DataDog/datadog-agent/pkg/util/cache"

var guidCacheKey = cache.BuildAgentKey("host", "utils", "uuid")
