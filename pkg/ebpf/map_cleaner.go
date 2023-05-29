// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
	"unsafe"

	cebpf "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/native"
)

// MapCleaner is responsible for periodically sweeping an eBPF map
// and deleting entries that satisfy a certain predicate function supplied by the user
type MapCleaner struct {
	emap *cebpf.Map
	key  interface{}
	val  interface{}
	once sync.Once

	// we resort to unsafe.Pointers because by doing so the underlying eBPF
	// library avoids marshaling the key/value variables while traversing the map
	keyPtr unsafe.Pointer
	valPtr unsafe.Pointer

	// termination
	stopOnce sync.Once
	done     chan struct{}
}

// NewMapCleaner instantiates a new MapCleaner
func NewMapCleaner(emap *cebpf.Map, key, val interface{}) (*MapCleaner, error) {
	// we force types to be of pointer kind because of the reasons mentioned above
	if reflect.ValueOf(key).Kind() != reflect.Ptr {
		return nil, fmt.Errorf("%T is not a pointer kind", key)
	}
	if reflect.ValueOf(val).Kind() != reflect.Ptr {
		return nil, fmt.Errorf("%T is not a pointer kind", val)
	}

	return &MapCleaner{
		emap:   emap,
		key:    key,
		val:    val,
		keyPtr: unsafe.Pointer(reflect.ValueOf(key).Elem().Addr().Pointer()),
		valPtr: unsafe.Pointer(reflect.ValueOf(val).Elem().Addr().Pointer()),
		done:   make(chan struct{}),
	}, nil
}

// Clean eBPF map
// `interval` determines how often the eBPF map is scanned;
// `shouldClean` is a predicate method that determines whether or not a certain
// map entry should be deleted. the callback argument `nowTS` can be directly
// compared to timestamps generated using the `bpf_ktime_get_ns()` helper;
func (mc *MapCleaner) Clean(interval time.Duration, shouldClean func(nowTS int64, k, v interface{}) bool) {
	if mc == nil {
		return
	}

	mc.once.Do(func() {
		ticker := time.NewTicker(interval)
		go func() {
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					now, err := NowNanoseconds()
					if err != nil {
						break
					}
					mc.clean(now, shouldClean)
				case <-mc.done:
					return
				}
			}

		}()
	})
}

// Stop stops the map cleaner
func (mc *MapCleaner) Stop() {
	if mc == nil {
		return
	}

	mc.stopOnce.Do(func() {
		// send to channel to synchronize with goroutine doing the map cleaning
		mc.done <- struct{}{}
		close(mc.done)
	})
}

func (mc *MapCleaner) clean(nowTS int64, shouldClean func(nowTS int64, k, v interface{}) bool) {
	keySize := int(mc.emap.KeySize())
	keysToDelete := make([][]byte, 0, 128)
	totalCount, deletedCount := 0, 0
	now := time.Now()

	entries := mc.emap.Iterate()
	for entries.Next(mc.keyPtr, mc.valPtr) {
		totalCount++

		if !shouldClean(nowTS, mc.key, mc.val) {
			continue
		}

		marshalledKey, err := marshalBytes(mc.key, keySize)
		if err != nil {
			continue
		}

		// we accumulate alll keys to delete because it isn't safe to delete map
		// entries during the traversal. the main downside of doing so is that all
		// fields from the key type must be exported in order to be marshaled (unless
		// the key type implements the `encoding.BinaryMarshaler` interface)
		keysToDelete = append(keysToDelete, marshalledKey)
	}

	for _, key := range keysToDelete {
		err := mc.emap.Delete(key)
		if err == nil {
			deletedCount++
		}
	}

	iterationErr := entries.Err()
	elapsed := time.Now().Sub(now)
	log.Debugf(
		"finished cleaning map=%s entries_checked=%d entries_deleted=%d iteration_error=%v elapsed=%s",
		mc.emap,
		totalCount,
		deletedCount,
		iterationErr,
		elapsed,
	)
}

// marshalBytes converts an arbitrary value into a byte buffer.
//
// Returns an error if the given value isn't representable in exactly
// length bytes.
//
// copied from: https://github.com/cilium/ebpf/blob/master/marshalers.go
func marshalBytes(data interface{}, length int) (buf []byte, err error) {
	if data == nil {
		return nil, errors.New("can't marshal a nil value")
	}

	switch value := data.(type) {
	case encoding.BinaryMarshaler:
		buf, err = value.MarshalBinary()
	case string:
		buf = []byte(value)
	case []byte:
		buf = value
	case unsafe.Pointer:
		err = errors.New("can't marshal from unsafe.Pointer")
	default:
		var wr bytes.Buffer
		err = binary.Write(&wr, native.Endian, value)
		if err != nil {
			err = fmt.Errorf("encoding %T: %v", value, err)
		}
		buf = wr.Bytes()
	}
	if err != nil {
		return nil, err
	}

	if len(buf) != length {
		return nil, fmt.Errorf("%T doesn't marshal to %d bytes", data, length)
	}
	return buf, nil
}
