// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"io"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

type PayloadItem interface {
	name() string
	GetTags() []string
}

type parseFunc[P PayloadItem] func(payload api.Payload) (items []P, err error)

type Aggregator[P PayloadItem] struct {
	payloadsByName map[string][]P
	parse          parseFunc[P]
}

const (
	encodingGzip    = "gzip"
	encodingDeflate = "deflate"
)

func newAggregator[P PayloadItem](parse parseFunc[P]) Aggregator[P] {
	return Aggregator[P]{
		payloadsByName: map[string][]P{},
		parse:          parse,
	}
}

func (agg *Aggregator[P]) UnmarshallPayloads(payloads []api.Payload) error {
	// reset map
	agg.payloadsByName = map[string][]P{}
	// build map
	for _, p := range payloads {
		payloads, err := agg.parse(p)
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
		if AreTagsSubsetOfOtherTags(tags, payloadItem.GetTags()) {
			return true
		}
	}

	return false
}

func enflate(payload []byte, encoding string) (enflated []byte, err error) {
	rc, err := getReadCloserForEncoding(payload, encoding)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	enflated, err = io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return enflated, nil
}

func getReadCloserForEncoding(payload []byte, encoding string) (rc io.ReadCloser, err error) {
	switch encoding {
	case encodingGzip:
		rc, err = gzip.NewReader(bytes.NewReader(payload))
	case encodingDeflate:
		rc, err = zlib.NewReader(bytes.NewReader(payload))
	default:
		rc = io.NopCloser(bytes.NewReader(payload))
	}
	return rc, err
}

func (agg *Aggregator[P]) GetPayloadsByName(name string) []P {
	return agg.payloadsByName[name]
}

func FilterByTags[P PayloadItem](payloads []P, tags []string) []P {
	ret := []P{}
	for _, p := range payloads {
		if AreTagsSubsetOfOtherTags(tags, p.GetTags()) {
			ret = append(ret, p)
		}
	}
	return ret
}

func AreTagsSubsetOfOtherTags(tags, otherTags []string) bool {
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
