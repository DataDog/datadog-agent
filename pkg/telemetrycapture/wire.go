// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetrycapture

import (
	"encoding/base64"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// jsonStream supports only the standard-library field types in our protocol.
// Unlike encoding/json it neither buffers entire records nor pools raw data.
// It is used only on the authenticated IPC reader, never on producer paths.
type jsonStream struct {
	writer io.Writer
	buffer [4096]byte
	n      int
	err    error
}

func (s *jsonStream) flush() {
	if s.err != nil || s.n == 0 {
		return
	}
	n, err := s.writer.Write(s.buffer[:s.n])
	if err == nil && n != s.n {
		err = io.ErrShortWrite
	}
	s.err = err
	clear(s.buffer[:s.n])
	s.n = 0
}

func (s *jsonStream) text(value string) {
	for len(value) != 0 && s.err == nil {
		n := copy(s.buffer[s.n:], value)
		s.n += n
		value = value[n:]
		if s.n == len(s.buffer) {
			s.flush()
		}
	}
}

func (s *jsonStream) Write(value []byte) (int, error) {
	size := len(value)
	for len(value) != 0 && s.err == nil {
		n := copy(s.buffer[s.n:], value)
		s.n += n
		value = value[n:]
		if s.n == len(s.buffer) {
			s.flush()
		}
	}
	return size - len(value), s.err
}

func (s *jsonStream) quoted(value string) {
	s.text(`"`)
	start := 0
	for i := 0; i < len(value); {
		c := value[i]
		if c >= utf8.RuneSelf {
			_, size := utf8.DecodeRuneInString(value[i:])
			if size == 1 {
				s.text(value[start:i])
				s.text(`\ufffd`)
				start = i + 1
			}
			i += size
			continue
		}
		if c == '"' || c == '\\' || c < 0x20 {
			s.text(value[start:i])
			switch c {
			case '"':
				s.text(`\"`)
			case '\\':
				s.text(`\\`)
			default:
				const hex = "0123456789abcdef"
				escaped := [6]byte{'\\', 'u', '0', '0', hex[c>>4], hex[c&15]}
				_, _ = s.Write(escaped[:])
			}
			start = i + 1
		}
		i++
	}
	s.text(value[start:])
	s.text(`"`)
}

func (s *jsonStream) value(value reflect.Value) {
	if s.err != nil {
		return
	}
	if value.Type() == reflect.TypeFor[time.Time]() {
		encoded, err := value.Interface().(time.Time).MarshalJSON()
		if err != nil {
			s.err = ErrRequest
			return
		}
		_, _ = s.Write(encoded)
		return
	}
	var number [32]byte
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			s.text("null")
			return
		}
		s.value(value.Elem())
	case reflect.String:
		s.quoted(value.String())
	case reflect.Bool:
		_, _ = s.Write(strconv.AppendBool(number[:0], value.Bool()))
	case reflect.Int, reflect.Int32, reflect.Int64:
		_, _ = s.Write(strconv.AppendInt(number[:0], value.Int(), 10))
	case reflect.Uint32, reflect.Uint64:
		_, _ = s.Write(strconv.AppendUint(number[:0], value.Uint(), 10))
	case reflect.Float64:
		n := value.Float()
		if math.IsNaN(n) || math.IsInf(n, 0) {
			s.err = ErrRequest
			return
		}
		_, _ = s.Write(strconv.AppendFloat(number[:0], n, 'g', -1, 64))
	case reflect.Struct:
		s.text("{")
		first := true
		for i := 0; i < value.NumField(); i++ {
			field := value.Type().Field(i)
			name, option, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				s.err = ErrRequest
				return
			}
			v := value.Field(i)
			if option == "omitempty" && emptyJSONValue(v) {
				continue
			}
			if !first {
				s.text(",")
			}
			first = false
			s.quoted(name)
			s.text(":")
			s.value(v)
		}
		s.text("}")
	case reflect.Slice:
		if value.IsNil() {
			s.text("null")
			return
		}
		if value.Type().Elem().Kind() == reflect.Uint8 {
			s.text(`"`)
			encoder := base64.NewEncoder(base64.StdEncoding, s)
			_, err := encoder.Write(value.Bytes())
			if err == nil {
				err = encoder.Close()
			}
			if err != nil {
				s.err = err
			}
			s.text(`"`)
			return
		}
		s.text("[")
		for i := 0; i < value.Len(); i++ {
			if i != 0 {
				s.text(",")
			}
			s.value(value.Index(i))
		}
		s.text("]")
	case reflect.Map:
		if value.IsNil() {
			s.text("null")
			return
		}
		if value.Type().Key().Kind() != reflect.String {
			s.err = ErrRequest
			return
		}
		s.text("{")
		iterator := value.MapRange()
		first := true
		for iterator.Next() {
			if !first {
				s.text(",")
			}
			first = false
			s.quoted(iterator.Key().String())
			s.text(":")
			s.value(iterator.Value())
		}
		s.text("}")
	default:
		s.err = ErrRequest
	}
}

func emptyJSONValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Slice, reflect.Map, reflect.String:
		return value.Len() == 0
	default:
		return value.IsZero()
	}
}
