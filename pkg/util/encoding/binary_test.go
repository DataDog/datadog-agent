// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package encoding

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type emptyTestType struct {
}

func (tt *emptyTestType) UnmarshalBinary(_ []byte) error {
	return nil
}

type errorTestType struct{}

func (tt *errorTestType) UnmarshalBinary(_ []byte) error {
	return errors.New("error")
}

type dataTestType struct {
	buf []byte
}

func (tt *dataTestType) UnmarshalBinary(data []byte) error {
	tt.buf = data
	return nil
}

func TestBinaryUnmarshalCallback(t *testing.T) {
	cb := BinaryUnmarshalCallback(func() *emptyTestType {
		return new(emptyTestType)
	}, func(x *emptyTestType, err error) {
		assert.Nil(t, x)
		assert.NoError(t, err)
	})
	cb(nil)
	cb([]byte{})

	cb = BinaryUnmarshalCallback(func() *errorTestType {
		return new(errorTestType)
	}, func(x *errorTestType, err error) {
		assert.NotNil(t, x)
		assert.Error(t, err)
	})
	cb([]byte{1, 2})

	cb = BinaryUnmarshalCallback(func() *dataTestType {
		return new(dataTestType)
	}, func(x *dataTestType, err error) {
		assert.Equal(t, []byte{1, 2}, x.buf)
		assert.NoError(t, err)
	})
	cb([]byte{1, 2})
}
