// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcscrape

import (
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type remoteConfigFile struct {
	RuntimeID     string
	ConfigPath    string
	ConfigContent string
}

// decoder handles the processing of the remote config probe output. It
// converts the raw data from the output.Event into a remoteConfigFile.
type decoder struct {
	ir        *ir.Program
	event     *ir.Event
	rootType  uint32
	dataItems map[dataItemKey]output.DataItem

	stringHeaderType *ir.GoStringHeaderType
	stringDataType   *ir.GoStringDataType
	strField         *ir.Field
	lenField         *ir.Field

	runtimeIDExpr     *ir.RootExpression
	configPathExpr    *ir.RootExpression
	configContentExpr *ir.RootExpression
}

// Don't spam the logs if dd-trace-go adds a new argument we don't expect.
var unknownArgumentRateLimiter = rate.NewLimiter(rate.Every(10*time.Second), 10)

// newDecoder creates a new decoder for the remote config probe.
func newDecoder(
	irp *ir.Program,
) (*decoder, error) {
	var rcEvent *ir.Event
	// Note that if a process has both v1 and v2, we're going to arbitrarily
	// prefer v1. In the future, we may want to support both simultaneously
	// and coalesce between them.
	for _, p := range irp.Probes {
		if p.GetID() != rcProbeIDV1 && p.GetID() != rcProbeIDV2 {
			continue
		}
		if len(p.Events) != 1 {
			return nil, fmt.Errorf(
				"remote config probe %q has %d events, expected 1",
				p.GetID(), len(p.Events),
			)
		}
		rcEvent = p.Events[0]
		break
	}

	if rcEvent == nil {
		return nil, fmt.Errorf("no remote config probe found in program")
	}
	if rcEvent.Type == nil {
		return nil, fmt.Errorf("remote config event has no type")
	}

	// All fields in the remote config event are expected to be strings.
	// We can probably relax this later if we need to.
	var (
		stringHeaderType                                 *ir.GoStringHeaderType
		runtimeIDExpr, configPathExpr, configContentExpr *ir.RootExpression
	)
	for _, expr := range rcEvent.Type.Expressions {
		switch expr.Name {
		case "runtimeID":
			runtimeIDExpr = expr
		case "configPath":
			configPathExpr = expr
		case "configContent":
			configContentExpr = expr
		default:
			if unknownArgumentRateLimiter.Allow() {
				log.Infof("ignoring unknown argument %q", expr.Name)
			}
		}
		ty := expr.Expression.Type
		t, ok := ty.(*ir.GoStringHeaderType)
		if !ok {
			kind, _ := ty.GetGoKind()
			return nil, fmt.Errorf(
				"argument %q is %v, not a go string header type (%s)",
				expr.Name, ty.GetName(), kind,
			)
		}
		if stringHeaderType == nil {
			stringHeaderType = t
		} else if stringHeaderType != t {
			return nil, fmt.Errorf(
				"remote config event uses multiple string header types: %s and %s",
				stringHeaderType.GetName(), t.GetName(),
			)
		}
	}
	if runtimeIDExpr == nil {
		return nil, fmt.Errorf("remote config event is missing runtimeID")
	}
	if configPathExpr == nil {
		return nil, fmt.Errorf("remote config event is missing configPath")
	}
	if configContentExpr == nil {
		return nil, fmt.Errorf("remote config event is missing configContent")
	}

	var (
		stringDataType     *ir.GoStringDataType
		strField, lenField *ir.Field
	)
	for i := range stringHeaderType.Fields {
		f := &stringHeaderType.Fields[i]
		switch f.Name {
		case "str":
			stringDataPtrType, ok := f.Type.(*ir.PointerType)
			if !ok {
				return nil, fmt.Errorf(
					"field %q in string header is not a pointer, %s is a %T",
					f.Name, f.Type.GetName(), f.Type,
				)
			}
			stringDataType, ok = stringDataPtrType.Pointee.(*ir.GoStringDataType)
			if !ok {
				return nil, fmt.Errorf(
					"field %q in string header is not pointing to a string data type, %s is a %T",
					f.Name, stringDataPtrType.Pointee.GetName(), stringDataPtrType.Pointee,
				)
			}
			strField = f
		case "len":
			lenType, ok := f.Type.(*ir.BaseType)
			if !ok {
				return nil, fmt.Errorf(
					"field %q in string header is not a go int data type, %s is a %T",
					f.Name, f.Type.GetName(), f.Type,
				)
			}
			if lenType.GetByteSize() != 8 {
				return nil, fmt.Errorf(
					"field %q in string header is not a 64-bit integer, %s is a %T",
					f.Name, f.Type.GetName(), f.Type,
				)
			}
			kind, _ := lenType.GetGoKind()
			if kind != reflect.Int {
				return nil, fmt.Errorf(
					"field %q in string header is not a go int data type, %s is a %s",
					f.Name, f.Type.GetName(), kind,
				)
			}
			lenField = f
		}
	}
	if strField == nil {
		return nil, fmt.Errorf("string header type is missing str field")
	}
	if lenField == nil {
		return nil, fmt.Errorf("string header type is missing len field")
	}

	return &decoder{
		ir:                irp,
		event:             rcEvent,
		rootType:          uint32(rcEvent.Type.ID),
		dataItems:         make(map[dataItemKey]output.DataItem),
		stringHeaderType:  stringHeaderType,
		stringDataType:    stringDataType,
		strField:          strField,
		lenField:          lenField,
		runtimeIDExpr:     runtimeIDExpr,
		configPathExpr:    configPathExpr,
		configContentExpr: configContentExpr,
	}, nil
}

type dataItemKey struct {
	typeID  uint32
	address uint64
}

var errNotRemoteConfig = errors.New("not a remote config event")

func (d *decoder) HandleMessage(ev output.Event) (_ remoteConfigFile, err error) {
	defer clear(d.dataItems)
	var (
		i          int
		rootHeader *output.DataItemHeader
		rootData   []byte
	)
	for dataItem, err := range ev.DataItems() {
		if err != nil {
			return remoteConfigFile{}, fmt.Errorf("error getting data item: %w", err)
		}
		if i == 0 {
			rootHeader = dataItem.Header()
			if rootHeader.Type != uint32(d.event.Type.ID) {
				return remoteConfigFile{}, errNotRemoteConfig
			}
			rootData = dataItem.Data()
		} else {
			header := dataItem.Header()
			key := dataItemKey{
				typeID:  header.Type,
				address: header.Address,
			}
			d.dataItems[key] = dataItem
		}
		i++
	}

	var configContentLen int
	var output remoteConfigFile
	stringLen := d.stringHeaderType.ByteSize
	for _, field := range d.event.Type.Expressions {
		if field.Expression.Type != d.stringHeaderType {
			return remoteConfigFile{}, fmt.Errorf(
				"field %s is not a string", field.Name,
			)
		}
		fieldOffset := int(field.Offset)
		if fieldOffset+int(stringLen) > len(rootData) {
			return remoteConfigFile{}, fmt.Errorf(
				"field %s is out of bounds", field.Name,
			)
		}
		fieldData := rootData[fieldOffset : fieldOffset+int(stringLen)]
		strAddr := binary.NativeEndian.Uint64(fieldData[d.strField.Offset:])
		strLen := binary.NativeEndian.Uint64(fieldData[d.lenField.Offset:])
		if strLen == 0 {
			continue
		}
		dataItem, ok := d.dataItems[dataItemKey{
			typeID:  uint32(d.stringDataType.ID),
			address: strAddr,
		}]
		if !ok {
			continue
		}
		strData := string(dataItem.Data())
		switch field.Name {
		case "runtimeID":
			output.RuntimeID = strData
		case "configPath":
			output.ConfigPath = strData
		case "configContent":
			configContentLen = int(strLen)
			output.ConfigContent = strData
		}
	}
	if configContentLen != len(output.ConfigContent) {
		log.Warnf(
			"runtimeID %q: configPath %q: configContent truncated: %d != %d",
			output.RuntimeID, output.ConfigPath,
			configContentLen, len(output.ConfigContent),
		)
		output.ConfigContent = ""
	}

	return output, nil
}
