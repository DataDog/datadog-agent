// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build systemd

package journald

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

const (
	tagSeparator            = ":"
	imageTagKey             = "short_image" + tagSeparator
	baseCacheExpiration     = 20 * time.Minute
	expirationSpreadSeconds = 120
)

// Get docker image short from dockerId and list of tags
// In case of miss in cache, we parse from tag list.
// We could use the util/Docker.Inspect + ImageID resolution or Regexp on config.Image
// But this way is simple and probably fast enough (~300ns for 100 items) on miss
func getDockerImageShortName(containerID string, tags []string) (string, bool) {
	cacheKey := getImageCacheKey(containerID)
	if cacheContent, hit := cache.Cache.Get(cacheKey); hit {
		// Even if we don't get the right type from the cache, we will override it anyway
		// The key is normally specific enough that only this process r/w the cache
		if value, castOk := cacheContent.(string); castOk {
			return value, true
		}
	}

	// Cache miss
	var shortName string
	for _, tag := range tags {
		if strings.HasPrefix(tag, imageTagKey) {
			tagParts := strings.Split(tag, tagSeparator)
			if len(tagParts) == 2 {
				shortName = tagParts[1]
			}
			break
		}
	}

	if shortName != "" {
		// We are using a cache as this function will be called for each entry in journald. On busy systems, looping to parse among a significant number of tags
		// could make the agent use a lot of CPU for nothing. To avoid CPU spikes when keys reach TTL all at the same time, we spread the TTL on several minutes
		cache.Cache.Set(cacheKey, shortName, baseCacheExpiration+time.Duration(rand.Intn(expirationSpreadSeconds))*time.Second)
		return shortName, true
	}

	return shortName, false
}

func getImageCacheKey(containerID string) string {
	return fmt.Sprintf("logger.tailer.imagefor.%s", containerID)
}
