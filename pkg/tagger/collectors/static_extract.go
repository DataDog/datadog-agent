// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package collectors

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (c *StaticCollector) getTagInfo(entity string) []*TagInfo {
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
	return tagInfoList
}
