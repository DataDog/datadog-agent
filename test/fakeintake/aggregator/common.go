// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"compress/zlib"
	"io/ioutil"
)

type GetPayloadResponse struct {
	Payloads [][]byte `json:"payloads"`
}

type PayloadItem interface {
	name() string
	tags() []string
}

type parseFunc[P PayloadItem] func(data []byte) (items []P, err error)

type Aggregator[P PayloadItem] struct {
	payloadsByName map[string][]P
	parse          parseFunc[P]
}

func newAggregator[P PayloadItem](parse parseFunc[P]) Aggregator[P] {
	return Aggregator[P]{
		payloadsByName: map[string][]P{},
		parse:          parse,
	}
}

func (agg *Aggregator[P]) UnmarshallPayloads(rawPayloads [][]byte) error {
	// reset map
	agg.payloadsByName = map[string][]P{}
	// build map
	for _, data := range rawPayloads {
		payloads, err := agg.parse(data)
		if err != nil {
			return err
		}
		for _, item := range payloads {
			if _, found := agg.payloadsByName[item.name()]; !found {
				agg.payloadsByName[item.name()] = []P{}
			}
			agg.payloadsByName[item.name()] = append(agg.payloadsByName[item.name()], item)
		}
	}

	return nil
}

func (agg *Aggregator[P]) ContainsPayloadName(name string) bool {
	_, found := agg.payloadsByName[name]
	return found
}

func (agg *Aggregator[P]) ContainsPayloadNameAndTags(name string, tags []string) bool {
	payloads, found := agg.payloadsByName[name]
	if !found {
		return false
	}

	for _, payloadItem := range payloads {
		if areTagsSubsetOfOtherTags(tags, payloadItem.tags()) {
			return true
		}
	}

	return false
}

func areTagsSubsetOfOtherTags(tags, otherTags []string) bool {
	otherTagsSet := tagsToSet(otherTags)
	for _, tag := range tags {
		if _, found := otherTagsSet[tag]; !found {
			return false
		}
	}
	return true
}

func tagsToSet(tags []string) map[string]struct{} {
	tagsSet := map[string]struct{}{}
	for _, tag := range tags {
		tagsSet[tag] = struct{}{}
	}
	return tagsSet
}

func enflate(payload []byte) (enflated []byte, err error) {
	re, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	enflated, err = ioutil.ReadAll(re)
	if err != nil {
		return nil, err
	}
	return enflated, nil
}
