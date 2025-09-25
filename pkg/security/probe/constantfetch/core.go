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

func (f *BTFConstantFetcher) getTypesForName(typeName string) ([]btf.Type, error) {
	actualTy := getActualTypeName(typeName)
	return f.spec.AnyTypesByName(actualTy)
}

func (f *BTFConstantFetcher) runSizeofRequest(typeName string) (uint64, error) {
	types, err := f.getTypesForName(typeName)
	if err != nil || len(types) == 0 {
		return ErrorSentinel, err
	}

	value := ErrorSentinel

	for _, ty := range types {
		sTy, ok := ty.(*btf.Struct)
		if ok {
			newValue := uint64(sTy.Size)
			if value != ErrorSentinel && value != newValue {
				return ErrorSentinel, errors.New("mismatching sizes in multiple BTF types")
			}
			value = newValue
		}

		uTy, ok := ty.(*btf.Union)
		if ok {
			newValue := uint64(uTy.Size)
			if value != ErrorSentinel && value != newValue {
				return ErrorSentinel, errors.New("mismatching sizes in multiple BTF types")
			}
			value = newValue
		}
	}

	return value, nil
}

// AppendSizeofRequest appends a sizeof request
func (f *BTFConstantFetcher) AppendSizeofRequest(id, typeName string) {
	value, err := f.runSizeofRequest(typeName)
	if err != nil {
		f.err = err
	}
	if value != ErrorSentinel {
		f.constants[id] = value
	}
}

func (f *BTFConstantFetcher) runOffsetofRequest(pairs ...TypeFieldPair) (uint64, error) {
	for _, pair := range pairs {
		types, err := f.getTypesForName(pair.TypeName)
		if err != nil || len(types) == 0 {
			return ErrorSentinel, err
		}

		for _, ty := range types {
			value := runOffsetofOnBTFType(pair.FieldName, ty)
			if value != ErrorSentinel {
				return value, nil
			}
		}
	}

	return ErrorSentinel, nil
}

// AppendOffsetofRequestWithFallbacks appends an offset request
func (f *BTFConstantFetcher) AppendOffsetofRequestWithFallbacks(id string, pairs ...TypeFieldPair) {
	value, err := f.runOffsetofRequest(pairs...)
	if err != nil {
		f.err = err
	}
	if value != ErrorSentinel {
		f.constants[id] = value
	}
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

func runOffsetofOnBTFType(fieldName string, ty btf.Type) uint64 {
	sTy, ok := ty.(*btf.Struct)
	if ok {
		return runOffsetofOnBTFTypeStructOrUnion(fieldName, sTy.Members)
	}

	uTy, ok := ty.(*btf.Union)
	if ok {
		return runOffsetofOnBTFTypeStructOrUnion(fieldName, uTy.Members)
	}

	return ErrorSentinel
}

func runOffsetofOnBTFTypeStructOrUnion(fieldName string, members []btf.Member) uint64 {
	for _, m := range members {
		if m.Name == "" {
			sub := runOffsetofOnBTFType(fieldName, m.Type)
			if sub != ErrorSentinel {
				return uint64(m.Offset.Bytes()) + sub
			}
		}

		if m.Name == fieldName {
			return uint64(m.Offset.Bytes())
		}
	}

	return ErrorSentinel
}
