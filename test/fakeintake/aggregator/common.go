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
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

//nolint:revive // TODO(APL) Fix revive linter
type PayloadItem interface {
	name() string
	GetTags() []string
	GetCollectedTime() time.Time
}

type parseFunc[P PayloadItem] func(payload api.Payload) (items []P, err error)

//nolint:revive // TODO(APL) Fix revive linter
type Aggregator[P PayloadItem] struct {
	payloadsByName map[string][]P
	parse          parseFunc[P]

	mutex sync.RWMutex
}

const (
	encodingGzip     = "gzip"
	encodingEmpty    = ""
	encodingJSON     = "application/json"
	encodingDeflate  = "deflate"
	encodingProtobuf = "protobuf"
)

func newAggregator[P PayloadItem](parse parseFunc[P]) Aggregator[P] {
	return Aggregator[P]{
		payloadsByName: map[string][]P{},
		parse:          parse,
		mutex:          sync.RWMutex{},
	}
}

// UnmarshallPayloads aggregate the payloads
func (agg *Aggregator[P]) UnmarshallPayloads(payloads []api.Payload) error {
	// build new map
	payloadsByName := map[string][]P{}
	for _, p := range payloads {
		payloads, err := agg.parse(p)
		if err != nil {
			return err
		}

		for _, item := range payloads {
			if _, found := payloadsByName[item.name()]; !found {
				payloadsByName[item.name()] = []P{}
			}
			payloadsByName[item.name()] = append(payloadsByName[item.name()], item)
		}
	}
	agg.replace(payloadsByName)

	return nil
}

// ContainsPayloadName return true if name match one of the payloads
func (agg *Aggregator[P]) ContainsPayloadName(name string) bool {
	return len(agg.GetPayloadsByName(name)) != 0
}

// ContainsPayloadNameAndTags return true if the payload name exist and on of the payloads contains all the tags
func (agg *Aggregator[P]) ContainsPayloadNameAndTags(name string, tags []string) bool {
	payloads := agg.GetPayloadsByName(name)

	for _, payloadItem := range payloads {
		if AreTagsSubsetOfOtherTags(tags, payloadItem.GetTags()) {
			return true
		}
	}

	return false
}

// GetNames return the names of the payloads
func (agg *Aggregator[P]) GetNames() []string {
	names := agg.getNamesUnsorted()
	sort.Strings(names)
	return names
}

func (agg *Aggregator[P]) getNamesUnsorted() []string {
	agg.mutex.RLock()
	defer agg.mutex.RUnlock()
	names := make([]string, 0, len(agg.payloadsByName))
	for name := range agg.payloadsByName {
		names = append(names, name)
	}
	return names
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

// GetPayloadsByName return the payloads for the resource name
func (agg *Aggregator[P]) GetPayloadsByName(name string) []P {
	agg.mutex.RLock()
	defer agg.mutex.RUnlock()
	payloads := agg.payloadsByName[name]
	return payloads
}

// Reset the aggregation
func (agg *Aggregator[P]) Reset() {
	agg.mutex.Lock()
	defer agg.mutex.Unlock()
	agg.unsafeReset()
}

func (agg *Aggregator[P]) unsafeReset() {
	agg.payloadsByName = map[string][]P{}
}

func (agg *Aggregator[P]) replace(payloadsByName map[string][]P) {
	agg.mutex.Lock()
	defer agg.mutex.Unlock()
	agg.unsafeReset()
	for name, payloads := range payloadsByName {
		agg.payloadsByName[name] = payloads
	}
}

// FilterByTags return the payloads that match all the tags
func FilterByTags[P PayloadItem](payloads []P, tags []string) []P {
	ret := []P{}
	for _, p := range payloads {
		if AreTagsSubsetOfOtherTags(tags, p.GetTags()) {
			ret = append(ret, p)
		}
	}
	return ret
}

// AreTagsSubsetOfOtherTags return true is all tags are in otherTags
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
