// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

// Package constantfetch holds constantfetch related files
package constantfetch

import (
	"errors"
	"io"
	"strings"

	"github.com/cilium/ebpf/btf"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// BTFConstantFetcher is a constant fetcher based on BTF data (from file or current kernel)
type BTFConstantFetcher struct {
	spec      *btf.Spec
	constants map[string]uint64
	err       error
}

// NewBTFConstantFetcherFromSpec creates a BTFConstantFetcher directly from a BTF spec
func NewBTFConstantFetcherFromSpec(spec *btf.Spec) *BTFConstantFetcher {
	return &BTFConstantFetcher{
		spec:      spec,
		constants: make(map[string]uint64),
	}
}

// NewBTFConstantFetcherFromReader creates a BTFConstantFetcher from a reader pointing to a BTF file
func NewBTFConstantFetcherFromReader(btfReader io.ReaderAt) (*BTFConstantFetcher, error) {
	spec, err := btf.LoadSpecFromReader(btfReader)
	if err != nil {
		return nil, err
	}
	return NewBTFConstantFetcherFromSpec(spec), nil
}

// NewBTFConstantFetcherFromCurrentKernel creates a BTFConstantFetcher, reading BTF from current kernel
func NewBTFConstantFetcherFromCurrentKernel() (*BTFConstantFetcher, error) {
	spec, err := ebpf.GetKernelSpec()
	if err != nil {
		return nil, err
	}
	return NewBTFConstantFetcherFromSpec(spec), nil
}

func (f *BTFConstantFetcher) String() string {
	return "btf co-re"
}

type constantRequest struct {
	id                  string
	sizeof              bool
	typeName, fieldName string
}

func (f *BTFConstantFetcher) runRequest(r constantRequest) {
	actualTy := getActualTypeName(r.typeName)
	types, err := f.spec.AnyTypesByName(actualTy)
	if err != nil || len(types) == 0 {
		// if it doesn't exist, we can't do anything
		return
	}

	finalValue := ErrorSentinel

	// the spec can contain multiple types for the same name
	// we check that they all return the same value for the same request
	for _, ty := range types {
		value := runRequestOnBTFType(r, ty)
		if value != ErrorSentinel {
			if finalValue != ErrorSentinel && finalValue != value {
				f.err = errors.New("mismatching values in multiple BTF types")
			}
			finalValue = value
		}
	}

	if finalValue != ErrorSentinel {
		f.constants[r.id] = finalValue
	}
}

// AppendSizeofRequest appends a sizeof request
func (f *BTFConstantFetcher) AppendSizeofRequest(id, typeName, headerName string) {
	f.runRequest(constantRequest{
		id:       id,
		sizeof:   true,
		typeName: getActualTypeName(typeName),
	})
}

// AppendOffsetofRequest appends an offset request
func (f *BTFConstantFetcher) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	f.runRequest(constantRequest{
		id:        id,
		sizeof:    false,
		typeName:  getActualTypeName(typeName),
		fieldName: fieldName,
	})
}

// FinishAndGetResults returns the results
func (f *BTFConstantFetcher) FinishAndGetResults() (map[string]uint64, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.constants, nil
}

func getActualTypeName(tn string) string {
	prefixes := []string{"struct", "enum"}
	for _, prefix := range prefixes {
		tn = strings.TrimPrefix(tn, prefix+" ")
	}
	return tn
}

func runRequestOnBTFType(r constantRequest, ty btf.Type) uint64 {
	sTy, ok := ty.(*btf.Struct)
	if !ok {
		return ErrorSentinel
	}

	if r.sizeof {
		return uint64(sTy.Size)
	}

	for _, m := range sTy.Members {
		if m.Name == r.fieldName {
			return uint64(m.Offset.Bytes())
		}
	}

	return ErrorSentinel
}
