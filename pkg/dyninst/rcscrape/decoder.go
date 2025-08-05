// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcscrape

import (
	"encoding/binary"
	"fmt"
	"reflect"

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
	dataItems map[dataItemKey]output.DataItem

	v1EventTypeID, v2EventTypeID, symdbEventTypeID ir.TypeID

	rcExprs    remoteConfigEventExpressions
	symdbExprs symdbEventExpressions
	stringDecoder
}

type eventDecoder interface {
	decodeEvent() // marker
}

func (*remoteConfigEventExpressions) decodeEvent() {}

func (d *decoder) getEventDecoder(ev output.Event) (eventDecoder, error) {
	header, err := ev.FirstDataItemHeader()
	if err != nil {
		return nil, err
	}
	switch ir.TypeID(header.Type) {
	case d.v1EventTypeID, d.v2EventTypeID:
		return (*remoteConfigEventDecoder)(d), nil
	case d.symdbEventTypeID:
		return (*symdbEventDecoder)(d), nil
	default:
		return nil, fmt.Errorf("unknown event type %d", header.Type)
	}
}

type remoteConfigEventDecoder decoder

type remoteConfigEventExpressions struct {
	runtimeIDExpr     *ir.RootExpression
	configPathExpr    *ir.RootExpression
	configContentExpr *ir.RootExpression
}

type symdbEventExpressions struct {
	runtimeIDExpr *ir.RootExpression
	enabledExpr   *ir.RootExpression
}

// newDecoder creates a new decoder for the remote config probes.
func newDecoder(irp *ir.Program) (*decoder, error) {
	// Note that if a process has both v1 and v2, we're going to arbitrarily
	// prefer v1. They are hard-coded to be identical with regards to their
	// types, so there's no problem with that.
	ret := decoder{
		dataItems: make(map[dataItemKey]output.DataItem),
	}
	var v1Event, v2Event, symdbEvent *ir.Event
	for _, p := range irp.Probes {
		var err error
		switch p.GetID() {
		case rcProbeIDV1:
			if v1Event, err = getFirstEvent(p); err != nil {
				return nil, fmt.Errorf("failed to get first event for v1 probe: %w", err)
			}
			ret.v1EventTypeID = v1Event.Type.ID
		case rcProbeIDV2:
			if v2Event, err = getFirstEvent(p); err != nil {
				return nil, fmt.Errorf("failed to get first event for v2 probe: %w", err)
			}
			ret.v2EventTypeID = v2Event.Type.ID
		case rcProbeIDSymdb:
			if symdbEvent, err = getFirstEvent(p); err != nil {
				return nil, fmt.Errorf("failed to get first event for symdb probe: %w", err)
			}
			ret.symdbEventTypeID = symdbEvent.Type.ID
		}
	}
	if v1Event == nil && v2Event == nil {
		return nil, fmt.Errorf("no remote config event found in program")
	}
	var rcFileEvent *ir.Event
	if v1Event != nil {
		rcFileEvent = v1Event
	} else {
		rcFileEvent = v2Event
	}

	var err error
	if ret.stringDecoder, err = makeStringDecoder(irp); err != nil {
		return nil, fmt.Errorf("failed to create string decoder: %w", err)
	}
	if ret.rcExprs, err = makeRemoteConfigFileExpressions(
		rcFileEvent, ret.stringDecoder.stringHeaderType,
	); err != nil {
		return nil, fmt.Errorf("failed to process remote config event: %w", err)
	}
	if symdbEvent != nil {
		if ret.symdbExprs, err = makeSymdbEventExpressions(
			symdbEvent, ret.stringDecoder.stringHeaderType,
		); err != nil {
			return nil, fmt.Errorf("failed to process symdb event: %w", err)
		}
	}
	return &ret, nil
}

func getFirstEvent(probe *ir.Probe) (*ir.Event, error) {
	if len(probe.Events) != 1 {
		return nil, fmt.Errorf(
			"probe %q has %d events, expected 1", probe.GetID(), len(probe.Events),
		)
	}
	return probe.Events[0], nil
}

func makeRemoteConfigFileExpressions(
	event *ir.Event,
	stringHeaderType *ir.GoStringHeaderType,
) (remoteConfigEventExpressions, error) {
	// All fields in the remote config event are expected to be strings.
	// We can probably relax this later if we need to.
	var runtimeIDExpr, configPathExpr, configContentExpr *ir.RootExpression
	for _, expr := range event.Type.Expressions {
		switch expr.Name {
		case "runtimeID":
			runtimeIDExpr = expr
		case "configPath":
			configPathExpr = expr
		case "configContent":
			configContentExpr = expr
		default:
			// We control the arguments we're passing in here, so we should know
			// what fields are expected.
			return remoteConfigEventExpressions{}, fmt.Errorf(
				"unknown field %q in remote config event", expr.Name,
			)
		}
		ty := expr.Expression.Type
		if ty != stringHeaderType {
			kind, _ := ty.GetGoKind()
			return remoteConfigEventExpressions{}, fmt.Errorf(
				"argument %q is %v, not a go string header type (%T; %v)",
				expr.Name, ty.GetName(), ty, kind,
			)
		}
	}
	if runtimeIDExpr == nil {
		return remoteConfigEventExpressions{}, fmt.Errorf(
			"remote config event is missing runtimeID",
		)
	}
	if configPathExpr == nil {
		return remoteConfigEventExpressions{}, fmt.Errorf(
			"remote config event is missing configPath",
		)
	}
	if configContentExpr == nil {
		return remoteConfigEventExpressions{}, fmt.Errorf(
			"remote config event is missing configContent",
		)
	}

	return remoteConfigEventExpressions{
		runtimeIDExpr:     runtimeIDExpr,
		configPathExpr:    configPathExpr,
		configContentExpr: configContentExpr,
	}, nil
}

func makeSymdbEventExpressions(
	event *ir.Event,
	stringHeaderType *ir.GoStringHeaderType,
) (symdbEventExpressions, error) {
	var runtimeIDExpr, enabledExpr *ir.RootExpression
	for _, expr := range event.Type.Expressions {
		switch expr.Name {
		case "runtimeID":
			runtimeIDExpr = expr
			if expr.Expression.Type != stringHeaderType {
				kind, _ := expr.Expression.Type.GetGoKind()
				return symdbEventExpressions{}, fmt.Errorf(
					"argument %q is %v, not a go string header type (%T; %v)",
					expr.Name, expr.Expression.Type.GetName(), expr.Expression.Type, kind,
				)
			}
		case "enabled":
			enabledExpr = expr
			if expr.Expression.Type != boolType {
				kind, _ := expr.Expression.Type.GetGoKind()
				return symdbEventExpressions{}, fmt.Errorf(
					"argument %q is %v, not a bool type (%T; %v)",
					expr.Name, expr.Expression.Type.GetName(), expr.Expression.Type, kind,
				)
			}
		default:
			return symdbEventExpressions{}, fmt.Errorf(
				"unknown field %q in symdb event", expr.Name,
			)
		}
	}
	if runtimeIDExpr == nil {
		return symdbEventExpressions{}, fmt.Errorf(
			"symdb event is missing runtimeID",
		)
	}
	if enabledExpr == nil {
		return symdbEventExpressions{}, fmt.Errorf(
			"symdb event is missing enabled",
		)
	}
	return symdbEventExpressions{
		runtimeIDExpr: runtimeIDExpr,
		enabledExpr:   enabledExpr,
	}, nil
}

type stringDecoder struct {
	stringHeaderType *ir.GoStringHeaderType
	stringDataType   *ir.GoStringDataType
	strField         *ir.Field
	lenField         *ir.Field
}

func makeStringDecoder(irp *ir.Program) (stringDecoder, error) {
	var stringHeaderType *ir.GoStringHeaderType
	for _, t := range irp.Types {
		var ok bool
		stringHeaderType, ok = t.(*ir.GoStringHeaderType)
		if ok {
			break
		}
	}
	if stringHeaderType == nil {
		return stringDecoder{}, fmt.Errorf("no string header type found in program")
	}

	var (
		stringDataType     *ir.GoStringDataType
		strField, lenField *ir.Field
	)
	for i := range stringHeaderType.RawFields {
		f := &stringHeaderType.RawFields[i]
		switch f.Name {
		case "str":
			stringDataPtrType, ok := f.Type.(*ir.PointerType)
			if !ok {
				return stringDecoder{}, fmt.Errorf(
					"field %q in string header is not a pointer, %s is a %T",
					f.Name, f.Type.GetName(), f.Type,
				)
			}
			stringDataType, ok = stringDataPtrType.Pointee.(*ir.GoStringDataType)
			if !ok {
				return stringDecoder{}, fmt.Errorf(
					"field %q in string header is not pointing to a string data type, %s is a %T",
					f.Name, stringDataPtrType.Pointee.GetName(), stringDataPtrType.Pointee,
				)
			}
			strField = f
		case "len":
			lenType, ok := f.Type.(*ir.BaseType)
			if !ok {
				return stringDecoder{}, fmt.Errorf(
					"field %q in string header is not a go int data type, %s is a %T",
					f.Name, f.Type.GetName(), f.Type,
				)
			}
			if lenType.GetByteSize() != 8 {
				return stringDecoder{}, fmt.Errorf(
					"field %q in string header is not a 64-bit integer, %s is a %T",
					f.Name, f.Type.GetName(), f.Type,
				)
			}
			kind, _ := lenType.GetGoKind()
			if kind != reflect.Int {
				return stringDecoder{}, fmt.Errorf(
					"field %q in string header is not a go int data type, %s is a %s",
					f.Name, f.Type.GetName(), kind,
				)
			}
			lenField = f
		}
	}
	if strField == nil {
		return stringDecoder{}, fmt.Errorf("string header type is missing str field")
	}
	if lenField == nil {
		return stringDecoder{}, fmt.Errorf("string header type is missing len field")
	}
	return stringDecoder{
		stringHeaderType: stringHeaderType,
		stringDataType:   stringDataType,
		strField:         strField,
		lenField:         lenField,
	}, nil
}

type dataItemKey struct {
	typeID  uint32
	address uint64
}

func processDataItems(
	dataItems map[dataItemKey]output.DataItem,
	ev output.Event,
) (rootData []byte, err error) {
	var i int
	for dataItem, err := range ev.DataItems() {
		if err != nil {
			return nil, fmt.Errorf("error getting data item: %w", err)
		}
		if i == 0 {
			rootData = dataItem.Data()
		} else {
			header := dataItem.Header()
			key := dataItemKey{
				typeID:  header.Type,
				address: header.Address,
			}
			dataItems[key] = dataItem
		}
		i++
	}
	return rootData, nil
}

func (d *stringDecoder) decodeStringExpression(
	expr *ir.RootExpression,
	rootData []byte,
	dataItems map[dataItemKey]output.DataItem,
) (str string, strLen int, err error) {
	fieldOffset := int(expr.Offset)
	if fieldOffset+int(d.stringHeaderType.ByteSize) > len(rootData) {
		return "", 0, fmt.Errorf("field %s is out of bounds", expr.Name)
	}
	fieldData := rootData[fieldOffset : fieldOffset+int(d.stringHeaderType.ByteSize)]

	strAddr := binary.NativeEndian.Uint64(fieldData[d.strField.Offset:])
	strLen = int(binary.NativeEndian.Uint64(fieldData[d.lenField.Offset:]))
	if strLen == 0 {
		return "", 0, nil
	}
	dataItem, ok := dataItems[dataItemKey{
		typeID:  uint32(d.stringDataType.ID),
		address: strAddr,
	}]
	if !ok {
		return "", 0, fmt.Errorf("string data item not found")
	}
	return string(dataItem.Data()), strLen, nil
}

func (d *remoteConfigEventDecoder) decodeEvent() {}

func (d *remoteConfigEventDecoder) decodeRemoteConfigFile(
	ev output.Event,
) (_ remoteConfigFile, err error) {
	defer clear(d.dataItems)

	rootData, err := processDataItems(d.dataItems, ev)
	if err != nil {
		return remoteConfigFile{}, fmt.Errorf(
			"error processing data items: %w", err,
		)
	}

	var output remoteConfigFile
	if output.RuntimeID, _, err = d.decodeStringExpression(
		d.rcExprs.runtimeIDExpr, rootData, d.dataItems,
	); err != nil {
		return remoteConfigFile{}, fmt.Errorf("error decoding runtimeID: %w", err)
	}
	if output.ConfigPath, _, err = d.decodeStringExpression(
		d.rcExprs.configPathExpr, rootData, d.dataItems,
	); err != nil {
		return remoteConfigFile{}, fmt.Errorf("error decoding configPath: %w", err)
	}
	var contentLen int
	if output.ConfigContent, contentLen, err = d.decodeStringExpression(
		d.rcExprs.configContentExpr, rootData, d.dataItems,
	); err != nil {
		return remoteConfigFile{}, fmt.Errorf("error decoding configContent: %w", err)
	}
	if contentLen != len(output.ConfigContent) {
		log.Warnf(
			"runtimeID %q: configPath %q: configContent truncated: %d != %d",
			output.RuntimeID, output.ConfigPath,
			contentLen, len(output.ConfigContent),
		)
		output.ConfigContent = ""
	}
	return output, nil
}

type symdbEventDecoder decoder

func (d *symdbEventDecoder) decodeEvent() {}

func (d *symdbEventDecoder) decodeSymdbEnabled(
	ev output.Event,
) (runtimeID string, symdbEnabled bool, err error) {
	defer clear(d.dataItems)

	rootData, err := processDataItems(d.dataItems, ev)
	if err != nil {
		return "", false, fmt.Errorf("error processing data items: %w", err)
	}

	if runtimeID, _, err = d.decodeStringExpression(
		d.symdbExprs.runtimeIDExpr, rootData, d.dataItems,
	); err != nil {
		return "", false, fmt.Errorf("error decoding runtimeID: %w", err)
	}
	log.Debugf("symdb enabled: %v %x %x", d.symdbExprs.enabledExpr.Offset, rootData, rootData[d.symdbExprs.enabledExpr.Offset:])
	symdbEnabledByte := rootData[d.symdbExprs.enabledExpr.Offset]
	symdbEnabled = symdbEnabledByte == 1
	return runtimeID, symdbEnabled, nil
}
