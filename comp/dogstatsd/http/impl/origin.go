// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	"fmt"
	"net/http"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

type tagsMap = map[string][]string

type origin struct {
	info taggertypes.OriginInfo
	tags tagsMap

	tagger tagger.Component
}

func originFromHeader(header http.Header, tagger tagger.Component) (origin, error) {
	localData, err := origindetection.ParseLocalData(header.Get("x-dsd-ld"))
	if err != nil {
		return origin{}, fmt.Errorf("failed to parse x-dsd-ld header: %w", err)
	}

	externalData, err := origindetection.ParseExternalData(header.Get("x-dsd-ed"))
	if err != nil {
		return origin{}, fmt.Errorf("failed to parse x-dsd-ed header: %w", err)
	}

	info := taggertypes.OriginInfo{
		LocalData:     localData,
		ExternalData:  externalData,
		ProductOrigin: origindetection.ProductOriginDogStatsD,
	}

	return origin{
		info:   info,
		tags:   make(tagsMap),
		tagger: tagger,
	}, nil
}

func (o *origin) getTags() []string {
	return o.getTagsWith("") // request default dogstatsd cardinality from config
}

func (o *origin) getTagsWith(card string) []string {
	tags, ok := o.tags[card]
	if !ok {
		acc := tagset.NewHashlessTagsAccumulator()
		info := o.info
		info.Cardinality = card
		o.tagger.EnrichTags(acc, info)
		tags = acc.Get()
		o.tags[card] = tags
	}

	return tags
}
