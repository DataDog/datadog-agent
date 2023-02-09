// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package payloadtesthelper

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io/ioutil"
)

type GetPayloadResponse struct {
	Payloads [][]byte `json:"payloads"`
}

type PayloadItem interface {
	name() string
}

type parseFunc[P PayloadItem] func(data []byte) (items []P, err error)

func unmarshallPayloads[P PayloadItem](body []byte, parse parseFunc[P]) (payloadsByName map[string][]P, err error) {
	payloadsByName = map[string][]P{}
	response := GetPayloadResponse{}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}

	// build filtered metric map
	for _, data := range response.Payloads {
		payloads, err := parse(data)
		if err != nil {
			return nil, err
		}
		for _, item := range payloads {
			if _, found := payloadsByName[item.name()]; !found {
				payloadsByName[item.name()] = []P{}
			}
			payloadsByName[item.name()] = append(payloadsByName[item.name()], item)
		}
	}
	return payloadsByName, nil
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
