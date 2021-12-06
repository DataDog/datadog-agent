// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pb

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
		assert.False(HasMetaHooks())
		assert.Nil(err)
		assert.Equal(map[string]string{"card.number": "4166 6766 6766 6746"}, s.Meta)
		b = newEmptyMessage()
		b = msgp.AppendString(b, "meta_struct")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "appsec")
		b = msgp.AppendBytes(b, msgp.AppendString(nil, "4166 6766 6766 6746"))
		s, err = decodeBytes(b)
		assert.False(HasMetaHooks())
		assert.Nil(err)
		assert.Equal(map[string][]byte{"appsec": msgp.AppendString(nil, "4166 6766 6766 6746")}, s.MetaStruct)
	})
	t.Run("meta", func(t *testing.T) {
		SetMetaHooks(func(k, v string) string { return "test" }, func(k string, v []byte) []byte { return msgp.AppendNil(nil) })
		defer SetMetaHooks(nil, nil)
		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "card.number")
		b = msgp.AppendString(b, "4166 6766 6766 6746")
		s, err := decodeBytes(b)
		assert := assert.New(t)
		assert.True(HasMetaHooks())
		assert.Nil(err)
		assert.Equal(map[string]string{"card.number": "test"}, s.Meta, "Warning! pkg/trace/pb: MetaHooks were not applied. One possible cause is regenerating the code in this folder without porting custom modifications of it.")
	})
	t.Run("meta_struct", func(t *testing.T) {
		SetMetaHooks(func(k, v string) string { return "test" }, func(k string, v []byte) []byte { return msgp.AppendNil(nil) })
		defer SetMetaHooks(nil, nil)
		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta_struct")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "appsec")
		b = msgp.AppendBytes(b, msgp.AppendString(nil, "4166 6766 6766 6746"))
		s, err := decodeBytes(b)
		assert := assert.New(t)
		assert.True(HasMetaHooks())
		assert.Nil(err)
		assert.True(msgp.IsNil(s.MetaStruct["appsec"]), "Warning! pkg/trace/pb: MetaHooks were not applied. One possible cause is regenerating the code in this folder without porting custom modifications of it.")
	})
	t.Run("both", func(t *testing.T) {
		SetMetaHooks(func(k, v string) string { return "test" }, func(k string, v []byte) []byte { return msgp.AppendNil(nil) })
		defer SetMetaHooks(nil, nil)
		b := newEmptyMessage()
		b = msgp.AppendString(b, "meta")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "card.number")
		b = msgp.AppendString(b, "4166 6766 6766 6746")
		s, err := decodeBytes(b)
		assert := assert.New(t)
		assert.True(HasMetaHooks())
		assert.Nil(err)
		assert.Equal(map[string]string{"card.number": "test"}, s.Meta, "Warning! pkg/trace/pb: MetaHooks were not applied. One possible cause is regenerating the code in this folder without porting custom modifications of it.")
		b = newEmptyMessage()
		b = msgp.AppendString(b, "meta_struct")
		b = msgp.AppendMapHeader(b, 1)
		b = msgp.AppendString(b, "appsec")
		b = msgp.AppendBytes(b, msgp.AppendString(nil, "4166 6766 6766 6746"))
		s, err = decodeBytes(b)
		assert.True(HasMetaHooks())
		assert.Nil(err)
		assert.True(msgp.IsNil(s.MetaStruct["appsec"]), "Warning! pkg/trace/pb: MetaHooks were not applied. One possible cause is regenerating the code in this folder without porting custom modifications of it.")
	})
}
