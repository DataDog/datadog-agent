// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These tests just provide coverage of the default stubs. Other tests
// perform more thorough validation of the functionality.

func TestEmptyTags(t *testing.T) {
	require.Equal(t, 0, EmptyTags.Len())
}

func TestNewTags(t *testing.T) {
	tags := NewTags([]string{"a", "b", "a"})
	tags.validate(t)
}

func TestNewUniqueTags(t *testing.T) {
	tags := NewUniqueTags("a", "b")
	tags.validate(t)
}

func TestNewTagsFromMap(t *testing.T) {
	tags := NewTagsFromMap(map[string]struct{}{"a": {}, "b": {}})
	tags.validate(t)
}

func TestNewBuilder(t *testing.T) {
	b := NewBuilder(10)
	b.Add("a")
	b.Add("b")
	tags := b.Close()
	tags.validate(t)
}

func TestUnion(t *testing.T) {
	tags := Union(
		NewTags([]string{"a", "b", "c"}),
		NewTags([]string{"c", "d", "e"}),
	)
	tags.validate(t)
}

func TestDefaultFactoryConcurrency(t *testing.T) {
	// fuzz test the threadsafety of this factory by doing a bunch of concurrent
	// operations and verifying things turn out OK
	fuzz(t, func(seed int64) {
		r := rand.New(rand.NewSource(seed))
		chans := make([]chan *Tags, 0)
		for i := 0; i < 10; i++ {
			ch := make(chan *Tags)
			chans = append(chans, ch)
			go func() {
				tags := NewTags([]string{fmt.Sprintf("tag%d", r.Intn(10))})
				ch <- tags
				defer func() {
					close(ch)
				}()
				for i := 0; i < 10; i++ {
					time.Sleep(time.Nanosecond * time.Duration(r.Intn(100)))
					switch r.Intn(6) {
					case 0:
						tags = NewTags([]string{fmt.Sprintf("tag%d", r.Intn(10))})
					case 1:
						tags = NewUniqueTags(fmt.Sprintf("tag%d", r.Intn(10)))
					case 2:
						tags = NewTag(fmt.Sprintf("tag%d", r.Intn(10)))
					case 3:
						tag2 := NewTag(fmt.Sprintf("tag%d", r.Intn(10)))
						tags = Union(tags, tag2)
					case 4:
						n := r.Intn(5)
						bldr := NewBuilder(4)
						bldr.AddTags(tags)
						for j := 0; j < n; j++ {
							bldr.Add(fmt.Sprintf("tag%d", r.Intn(10)))
						}
						tags = bldr.Close()
					case 5:
						n := r.Intn(8)
						bldr := NewSliceBuilder(2, 4)
						for j := 0; j < n; j++ {
							bldr.Add(r.Intn(2), fmt.Sprintf("tag%d", r.Intn(10)))
						}
						tags = bldr.FreezeSlice(0, 1)
					}
					ch <- tags
				}
			}()
		}

		// validate the tags in the main goroutine (testing.T cannot be used in other goroutines)
		for _, ch := range chans {
			for tags := range ch {
				tags.validate(t)
			}
		}
	})
}
