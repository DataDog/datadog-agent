// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tinylib/msgp/msgp"
)

func TestMetaHook(t *testing.T) {
	t.Run("off", func(t *testing.T) {
		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "card.number")
		b = msgp.AppendString(b, "4166 6766 6766 6746")
		s, err := decodeBytes(b)

		assert := assert.New(t)
		assert.Nil(err)
		assert.Equal(map[string]string{"card.number": "4166 6766 6766 6746"}, s.Meta)
	})

	t.Run("on", func(t *testing.T) {
		SetMetaHook(func(k, v string) string { return "test" })
		defer SetMetaHook(nil)

		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "card.number")
		b = msgp.AppendString(b, "4166 6766 6766 6746")
		s, err := decodeBytes(b)

		assert := assert.New(t)
		assert.Nil(err)
		assert.Equal(map[string]string{"card.number": "test"}, s.Meta, "Warning! pkg/proto/pbgo/trace: MetaHook was not applied. One possible cause is regenerating the code in this folder without porting custom modifications of it.")
	})
}
