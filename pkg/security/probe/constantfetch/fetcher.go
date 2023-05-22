// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package constantfetch

import (
	"crypto/md5"
	"fmt"
	"hash"
	"io"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// ErrorSentinel is the value of an unavailable offset or size
const ErrorSentinel uint64 = ^uint64(0)

// ConstantFetcher represents a source of constants that can be used to fill up
// eBPF relocations
type ConstantFetcher interface {
	fmt.Stringer
	AppendSizeofRequest(id, typeName, headerName string)
	AppendOffsetofRequest(id, typeName, fieldName, headerName string)
	FinishAndGetResults() (map[string]uint64, error)
}

// ComposeConstantFetcher represents a composition of child constant fetchers
// It allows the usage of fallbacks if the main source is not working
type ComposeConstantFetcher struct {
	hasher   hash.Hash
	fetchers []ConstantFetcher
	requests []*composeRequest
}

// ComposeConstantFetchers creates a new ComposeConstantFetcher based on the
// passed fetchers
func ComposeConstantFetchers(fetchers []ConstantFetcher) *ComposeConstantFetcher {
	return &ComposeConstantFetcher{
		hasher:   md5.New(),
		fetchers: fetchers,
	}
}

func (f *ComposeConstantFetcher) String() string {
	return fmt.Sprintf("composition of %s", f.fetchers)
}

func (f *ComposeConstantFetcher) appendRequest(req *composeRequest) {
	f.requests = append(f.requests, req)
	_, _ = io.WriteString(f.hasher, req.id)
	_, _ = io.WriteString(f.hasher, req.typeName)
	_, _ = io.WriteString(f.hasher, req.fieldName)
	_, _ = io.WriteString(f.hasher, req.headerName)
}

// AppendSizeofRequest appends a sizeof request
func (f *ComposeConstantFetcher) AppendSizeofRequest(id, typeName, headerName string) {
	f.appendRequest(&composeRequest{
		id:         id,
		sizeof:     true,
		typeName:   typeName,
		fieldName:  "",
		headerName: headerName,
		value:      ErrorSentinel,
	})
}

// AppendOffsetofRequest appends an offset request
func (f *ComposeConstantFetcher) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	f.appendRequest(&composeRequest{
		id:         id,
		sizeof:     false,
		typeName:   typeName,
		fieldName:  fieldName,
		headerName: headerName,
		value:      ErrorSentinel,
	})
}

func (f *ComposeConstantFetcher) getHash() []byte {
	return f.hasher.Sum(nil)
}

func (f *ComposeConstantFetcher) fillConstantCacheIfNeeded() {
	currentHash := f.getHash()
	if constantsCache.isMatching(currentHash) {
		return
	}

	for _, fetcher := range f.fetchers {
		for _, req := range f.requests {
			if req.value == ErrorSentinel {
				if req.sizeof {
					fetcher.AppendSizeofRequest(req.id, req.typeName, req.headerName)
				} else {
					fetcher.AppendOffsetofRequest(req.id, req.typeName, req.fieldName, req.headerName)
				}
			}
		}

		res, err := fetcher.FinishAndGetResults()
		if err != nil {
			seclog.Errorf("failed to run constant fetcher: %v", err)
		}

		for _, req := range f.requests {
			if req.value == ErrorSentinel {
				if newValue, present := res[req.id]; present {
					req.value = newValue
					req.fetcherName = fetcher.String()
				}
			}
		}
	}

	finalRes := make(map[string]ValueAndSource)
	for _, req := range f.requests {
		finalRes[req.id] = ValueAndSource{
			ID:          req.id,
			Value:       req.value,
			FetcherName: req.fetcherName,
		}
	}

	constantsCache = &cachedConstants{
		constants: finalRes,
		hash:      currentHash,
	}
}

// FinishAndGetResults does the actual fetching and returns the results
func (f *ComposeConstantFetcher) FinishAndGetResults() (map[string]uint64, error) {
	f.fillConstantCacheIfNeeded()
	return constantsCache.getConstants(), nil
}

// FinishAndGetStatus does the actual fetching and returns the status
func (f *ComposeConstantFetcher) FinishAndGetStatus() (*ConstantFetcherStatus, error) {
	f.fillConstantCacheIfNeeded()

	fetcherNames := make([]string, 0, len(f.fetchers))
	for _, fetcher := range f.fetchers {
		fetcherNames = append(fetcherNames, fetcher.String())
	}

	return &ConstantFetcherStatus{
		Fetchers: fetcherNames,
		Values:   constantsCache.constants,
	}, nil
}

// ConstantFetcherStatus represents the status of the constant fetcher sub-system
type ConstantFetcherStatus struct {
	Fetchers []string
	Values   map[string]ValueAndSource
}

type composeRequest struct {
	id                  string
	sizeof              bool
	typeName, fieldName string
	headerName          string
	value               uint64
	fetcherName         string
}

// CreateConstantEditors creates constant editors based on the constants fetched
func CreateConstantEditors(constants map[string]uint64) []manager.ConstantEditor {
	res := make([]manager.ConstantEditor, 0, len(constants))
	for name, value := range constants {
		if value == ErrorSentinel {
			seclog.Errorf("failed to fetch constant for %s", name)
			value = 0
		}

		res = append(res, manager.ConstantEditor{
			Name:  name,
			Value: value,
		})
	}
	return res
}

var constantsCache *cachedConstants

// ClearConstantsCache clears the constants cache
func ClearConstantsCache() {
	constantsCache = nil
}

// ValueAndSource represents the required information about a constant, its id, its value and its source
type ValueAndSource struct {
	ID          string
	Value       uint64
	FetcherName string
}

type cachedConstants struct {
	constants map[string]ValueAndSource
	hash      []byte
}

func (cc *cachedConstants) isMatching(hash []byte) bool {
	if cc == nil {
		return false
	}

	if len(hash) != len(cc.hash) {
		return false
	}

	for i, v := range cc.hash {
		if v != hash[i] {
			return false
		}
	}
	return true
}

func (cc *cachedConstants) getConstants() map[string]uint64 {
	res := make(map[string]uint64)
	for k, v := range cc.constants {
		res[k] = v.Value
	}
	return res
}
