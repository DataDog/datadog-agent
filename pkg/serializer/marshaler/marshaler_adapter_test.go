// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshaler

import (
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"
)

func TestIterableStreamJSONMarshalerAdapter(t *testing.T) {
	m := &DummyMarshaller{
		Items:  []string{"item1", "item2", "item3"},
		Header: "header",
		Footer: "footer",
	}
	marshaler := NewIterableStreamJSONMarshalerAdapter(m)
	stream := jsoniter.NewStream(jsoniter.ConfigDefault, nil, 0)

	r := require.New(t)
	r.NoError(marshaler.WriteHeader(stream))
	for marshaler.MoveNext() {
		r.NoError(marshaler.WriteCurrentItem(stream))
	}
	r.NoError(marshaler.WriteFooter(stream))
	require.Equal(t, "headeritem1item2item3footer", string(stream.Buffer()))
}
