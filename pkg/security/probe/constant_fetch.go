// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import "github.com/DataDog/datadog-agent/pkg/security/log"

const errorSentinel uint64 = ^uint64(0)

type ConstantFetcher interface {
	AppendSizeofRequest(id, typeName, headerName string)
	AppendOffsetofRequest(id, typeName, fieldName, headerName string)
	FinishAndGetResults() (map[string]uint64, error)
}

type ComposeConstantFetcher struct {
	fetchers []ConstantFetcher
	requests []*composeRequest
}

func ComposeConstantFetchers(fetchers []ConstantFetcher) *ComposeConstantFetcher {
	return &ComposeConstantFetcher{
		fetchers: fetchers,
	}
}

func (f *ComposeConstantFetcher) appendRequest(req *composeRequest) {
	f.requests = append(f.requests, req)
}

func (f *ComposeConstantFetcher) AppendSizeofRequest(id, typeName, headerName string) {
	f.appendRequest(&composeRequest{
		id:         id,
		sizeof:     true,
		typeName:   typeName,
		fieldName:  "",
		headerName: headerName,
		value:      errorSentinel,
	})
}

func (f *ComposeConstantFetcher) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	f.appendRequest(&composeRequest{
		id:         id,
		sizeof:     false,
		typeName:   typeName,
		fieldName:  fieldName,
		headerName: headerName,
		value:      errorSentinel,
	})
}

func (f *ComposeConstantFetcher) FinishAndGetResults() (map[string]uint64, error) {
	for _, fetcher := range f.fetchers {
		for _, req := range f.requests {
			if req.value == errorSentinel {
				if req.sizeof {
					fetcher.AppendSizeofRequest(req.id, req.typeName, req.headerName)
				} else {
					fetcher.AppendOffsetofRequest(req.id, req.typeName, req.fieldName, req.headerName)
				}
			}
		}

		res, err := fetcher.FinishAndGetResults()
		if err != nil {
			log.Errorf("failed to run constant fetcher: %v", err)
		}

		for _, req := range f.requests {
			if req.value == errorSentinel {
				if newValue, present := res[req.id]; present {
					req.value = newValue
				}
			}
		}
	}

	finalRes := make(map[string]uint64)
	for _, req := range f.requests {
		finalRes[req.id] = req.value
	}
	return finalRes, nil
}

type composeRequest struct {
	id                  string
	sizeof              bool
	typeName, fieldName string
	headerName          string
	value               uint64
}

type FallbackConstantFetcher struct {
	probe *Probe
	res   map[string]uint64
}

func NewFallbackConstantFetcher(probe *Probe) *FallbackConstantFetcher {
	return &FallbackConstantFetcher{
		probe: probe,
		res:   make(map[string]uint64),
	}
}

func (f *FallbackConstantFetcher) appendRequest(id string) {
	var value uint64 = errorSentinel
	switch id {
	case "sizeof_inode":
		value = getSizeOfStructInode(f.probe)
	case "sb_magic_offset":
		value = getSuperBlockMagicOffset(f.probe)
	case "tty_offset":
		value = getSignalTTYOffset(f.probe)
	case "tty_name_offset":
		value = getTTYNameOffset(f.probe)
	}
	f.res[id] = value
}

func (f *FallbackConstantFetcher) AppendSizeofRequest(id, typeName, headerName string) {
	f.appendRequest(id)
}

func (f *FallbackConstantFetcher) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	f.appendRequest(id)
}

func (f *FallbackConstantFetcher) FinishAndGetResults() (map[string]uint64, error) {
	return f.res, nil
}
