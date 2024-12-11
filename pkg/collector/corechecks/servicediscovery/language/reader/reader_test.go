// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package reader

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var errRead = errors.New("expected error")

type readerError struct{}

func (r readerError) Read(_ []byte) (n int, err error) {
	return 0, errRead
}

func TestIndex_error(t *testing.T) {
	r := readerError{}
	_, err := Index(r, "a")
	if !errors.Is(err, errRead) {
		t.Errorf("expected %v, got %v", errRead, err)
	}
}

func TestIndex_long(t *testing.T) {
	toFind := strings.Repeat("a", stringReaderBufferSize+10)
	source := strings.NewReader("asdfasdfasdf" + toFind + "asdfasdfasdf")
	offset, err := Index(source, toFind)
	if err != nil {
		t.Error(err)
	}
	if offset != 12 {
		t.Error("expected 12, got", offset)
	}
}

func TestIndex(t *testing.T) {
	toMatch := `testing`

	// handle missing case
	t.Run("missing", func(t *testing.T) {
		in := make([]byte, 2*stringReaderBufferSize+10)
		r := bytes.NewBuffer(in)
		offset, err := Index(r, toMatch)
		if err != nil {
			t.Error(err)
		}
		if offset != -1 {
			t.Error("expected -1, got", offset)
		}
	})

	data := []struct {
		name   string
		offset int
	}{
		{"1", stringReaderBufferSize},
		{"2", 2 * stringReaderBufferSize},
	}
	for _, d := range data {
		// write toMatch at various positions across the buffer boundary for the function
		// and check that it is found
		for j := -7; j <= 2; j++ {
			t.Run(fmt.Sprintf("%s:%d", d.name, j), func(t *testing.T) {
				in := make([]byte, 2*stringReaderBufferSize+10)
				// get away with this because toMatch is all ascii
				for i, v := range toMatch {
					in[i+d.offset+j] = byte(v)
				}
				r := bytes.NewBuffer(in)
				offset, err := Index(r, toMatch)
				if err != nil {
					t.Error(err)
				}
				if offset != d.offset+j {
					t.Error("expected", d.offset+j, "got", offset)
				}
			})
		}
	}
}

func Test_findPrefixAtEnd(t *testing.T) {
	data := []struct {
		name     string
		buf      []byte
		toFind   []byte
		expected []byte
	}{
		{"same", []byte(`hello`), []byte(`hello`), []byte(`hello`)},
		{"short", []byte(`a`), []byte(`hello`), nil},
		{"nils", nil, nil, nil},
		{"partial", []byte(`this is a test`), []byte(`testing`), []byte(`test`)},
		{"shorter", []byte(`test`), []byte(`testing`), []byte(`test`)},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			result := findPrefixAtEnd(d.buf, d.toFind)
			if diff := cmp.Diff(result, d.expected); diff != "" {
				t.Error(diff)
			}
		})
	}
}
