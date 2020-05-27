// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build docker kubelet

package collectors

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	staticCollectorName = "static"
	staticExpireFreq    = 5 * time.Minute
)

// StaticCollector fetches "static" tags, e.g. those from an env var
// It is not intended to run as a stand alone
type StaticCollector struct {
	infoOut      chan<- []*TagInfo
	expire       *utils.Expire
	lastExpire   time.Time
	expireFreq   time.Duration
	ddTagsEnvVar []string
}

func (c *StaticCollector) Detect(out chan<- []*TagInfo) (CollectionMode, error) {
	var err error
	c.infoOut = out
	c.expire, err = utils.NewExpire(staticExpireFreq)
	c.lastExpire = time.Now()
	c.expireFreq = staticExpireFreq
	// Extract DD_TAGS environment variable
	c.ddTagsEnvVar = config.Datadog.GetStringSlice("tags")

	if err != nil {
		return FetchOnlyCollection, fmt.Errorf("Failed to instantiate the expiration process")
	}
	return FetchOnlyCollection, nil
}

// Fetch fetches static tags
func (c *StaticCollector) Fetch(entity string) ([]string, []string, []string, error) {
	tags := utils.NewTagList()
	for _, tag := range c.ddTagsEnvVar {
		tagParts := strings.SplitN(tag, ":", 2)
		if len(tagParts) != 2 {
			log.Warnf("Cannot split tag %s", tag)
			continue
		}
		tags.AddLow(tagParts[0], tagParts[1])
	}

	lowTags, _, _ := tags.Compute()

	var tagInfoList []*TagInfo

	tagInfo := &TagInfo{
		Source:      staticCollectorName,
		Entity:      entity,
		LowCardTags: lowTags,
	}

	tagInfoList = append(tagInfoList, tagInfo)

	c.infoOut <- tagInfoList

	// Use a go routine to mark expires as the expire process can be done asynchronously.
	// We do not need the output as the StaticCollector is not meant run as a standalone and another
	// collector can handle entity pruning in the tagStore.
	if time.Now().Sub(c.lastExpire) >= c.expireFreq {
		go c.expire.ComputeExpires()
		c.lastExpire = time.Now()
	}

	return c.ddTagsEnvVar, []string{}, []string{}, nil
}

func staticFactory() Collector {
	return &StaticCollector{}
}

func init() {
	registerCollector(staticCollectorName, staticFactory, NodeOrchestrator)
}
