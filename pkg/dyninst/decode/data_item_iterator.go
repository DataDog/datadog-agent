// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package decode

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

const (
	eventHeaderSize    = int(unsafe.Sizeof(output.EventHeader{}))
	dataItemHeaderSize = int(unsafe.Sizeof(output.DataItemHeader{}))
)

var finishedIterating = errors.New("no data items left to iterate")

type dataItem struct {
	header *output.DataItemHeader
	data   []byte
}

type dataEventIterator struct {
	buf            []byte
	eventHeader    *output.EventHeader
	idx            int
	addressPassMap map[uint64]*dataItem
}

func init() {
	if dataItemHeaderSize == 0 || eventHeaderSize == 0 {
		panic("invalid header size for decoding buffers")
	}
}

// newDataEventIterator creates a new data event iterator.
//
// It returns a pointer to the iterator.
func newDataEventIterator(buf []byte) (*dataEventIterator, error) {
	iterator := &dataEventIterator{
		addressPassMap: make(map[uint64]*dataItem),
	}
	iterator.setBuffer(buf)
	err := iterator.setEventHeader()
	if err != nil {
		return nil, err
	}
	err = iterator.populateReferenceMap()
	if err != nil {
		return nil, err
	}

	return iterator, nil
}

// setBuffer sets the buffer to be used by the iterator.
//
// It returns an error if there are not enough bytes to read the event header.
// The passed buffer is NOT copied into the iterator's internal buffer so the caller
// must be careful to avoid issues with modifying the buffer while the iterator is
// reading from it.
func (e *dataEventIterator) setBuffer(buf []byte) error {
	if e == nil {
		return errors.New("nil iterator")
	}
	if len(buf) < int(unsafe.Sizeof(output.EventHeader{})) {
		return errors.New("not enough bytes to read event header")
	}
	e.buf = buf
	e.idx = 0
	e.eventHeader = nil
	return nil
}

// setEventHeader reads the event header from the buffer.
//
// The event header is set in the iterator's internal representation.
func (e *dataEventIterator) setEventHeader() error {
	if e == nil {
		return errors.New("nil iterator")
	}
	if e.eventHeader != nil {
		return nil
	}
	if e.idx != 0 {
		return errors.New("event header already read, but not set")
	}

	eventHeaderSize := int(unsafe.Sizeof(output.EventHeader{}))
	if len(e.buf) < eventHeaderSize {
		return errors.New("not enough bytes to read event header")
	}
	e.eventHeader = (*output.EventHeader)(unsafe.Pointer(&e.buf[0]))
	e.idx = eventHeaderSize
	return nil
}

// nextDataItem reads the next data item from the buffer.
//
// It returns a pointer to the data item and an error if there are not enough
// bytes to read the data item header or data item.
func (e *dataEventIterator) nextDataItem() (*dataItem, error) {
	if e == nil {
		return nil, errors.New("nil iterator")
	}

	// Read the data item header if there are enough bytes
	if e.idx+dataItemHeaderSize >= len(e.buf) {
		return nil, finishedIterating
	}
	header := (*output.DataItemHeader)(unsafe.Pointer(&e.buf[e.idx]))
	e.idx += dataItemHeaderSize

	// Read the data item if there are enough bytes
	if e.idx+int(header.Length) > len(e.buf) {
		return nil, errors.New("not enough bytes to read data item: ")
	}
	data := e.buf[e.idx : e.idx+int(header.Length)]
	e.idx += int(header.Length)

	return &dataItem{
		header: header,
		data:   data,
	}, nil
}

func (e *dataEventIterator) populateReferenceMap() error {
	if e == nil {
		return errors.New("nil iterator")
	}
	saveIndex := e.idx
	_, err := e.nextDataItem()
	if err != nil && !errors.Is(err, finishedIterating) {
		return fmt.Errorf("could not get first root data item from buffer for populating reference map: %s", err)
	}

	// Populate the reference map which keeps track of pointer data items.
	referenceMap := map[uint64]*dataItem{}
	for {
		dataItem, err := e.nextDataItem()
		if errors.Is(err, finishedIterating) {
			break
		}
		if err != nil {
			return err
		}
		if dataItem.header.Address != 0 {
			referenceMap[dataItem.header.Address] = dataItem
		}
	}
	e.idx = saveIndex
	e.addressPassMap = referenceMap
	return nil
}
