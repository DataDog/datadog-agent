// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"encoding"
	"encoding/hex"

	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	manager "github.com/DataDog/ebpf-manager"
)

type kFilter interface {
	Remove(*manager.Manager) error
	Apply(*manager.Manager) error
	Key() interface{}
	GetTableName() string
	GetApproverType() string
}

// KFilters defines kfilter map
type KFilters map[any]kFilter

func newKFilters(kfilters ...kFilter) (ak KFilters) {
	ak = make(KFilters)
	for _, kfilter := range kfilters {
		if kfilter != nil {
			ak.Add(kfilter)
		}
	}
	return
}

// HasKey returns if a filter exists
func (ak KFilters) HasKey(key any) bool {
	_, found := ak[key]
	return found
}

// Sub remove filters of the given filters
func (ak KFilters) Sub(ak2 KFilters) {
	for key := range ak {
		if _, found := ak2[key]; found {
			delete(ak, key)
		}
	}
}

// Add a filter
func (ak KFilters) Add(a kFilter) {
	ak[a.Key()] = a
}

// Remove a filter
func (ak KFilters) Remove(a kFilter) {
	delete(ak, a.Key())
}

type kFilterKey struct {
	tableName string
	key       any
}

func makeKFilterKey(tableName string, tableKey any) kFilterKey {
	mb, ok := tableKey.(encoding.BinaryMarshaler)
	if !ok {
		return kFilterKey{
			tableName: tableName,
			key:       tableKey,
		}
	}

	data, _ := mb.MarshalBinary()

	return kFilterKey{
		tableName: tableName,
		key:       hex.EncodeToString(data),
	}
}

type arrayKFilter struct {
	approverType string
	tableName    string
	index        any
	value        any
	zeroValue    any
}

func (e *arrayKFilter) Key() any {
	return makeKFilterKey(e.tableName, e.index)
}

func (e *arrayKFilter) GetTableName() string {
	return e.tableName
}

func (e *arrayKFilter) GetApproverType() string {
	return e.approverType
}

func (e *arrayKFilter) Remove(manager *manager.Manager) error {
	table, err := managerhelper.Map(manager, e.tableName)
	if err != nil {
		return err
	}
	return table.Put(e.index, e.zeroValue)
}

func (e *arrayKFilter) Apply(manager *manager.Manager) error {
	table, err := managerhelper.Map(manager, e.tableName)
	if err != nil {
		return err
	}
	return table.Put(e.index, e.value)
}

type eventMaskKFilter struct {
	approverType string
	tableName    string
	tableKey     any
	eventMask    uint64
}

func (e *eventMaskKFilter) Key() any {
	return makeKFilterKey(e.tableName, e.tableKey)
}

func (e *eventMaskKFilter) GetTableName() string {
	return e.tableName
}

func (e *eventMaskKFilter) GetApproverType() string {
	return e.approverType
}

func (e *eventMaskKFilter) Remove(manager *manager.Manager) error {
	table, err := managerhelper.Map(manager, e.tableName)
	if err != nil {
		return err
	}
	var eventMask uint64
	if err := table.Lookup(e.tableKey, &eventMask); err != nil {
		return err
	}
	if eventMask &^= e.eventMask; eventMask == 0 {
		return table.Delete(e.tableKey)
	}
	return table.Put(e.tableKey, eventMask)
}

func (e *eventMaskKFilter) Apply(manager *manager.Manager) error {
	table, err := managerhelper.Map(manager, e.tableName)
	if err != nil {
		return err
	}
	var eventMask uint64
	_ = table.Lookup(e.tableKey, &eventMask)
	if eventMask |= e.eventMask; eventMask == 0 {
		return table.Delete(e.tableKey)
	}
	return table.Put(e.tableKey, eventMask)
}

type hashKFilter struct {
	approverType string
	tableName    string
	tableKey     any
	value        any
}

func (e *hashKFilter) Key() any {
	return makeKFilterKey(e.tableName, e.tableKey)
}

func (e *hashKFilter) GetTableName() string {
	return e.tableName
}

func (e *hashKFilter) GetApproverType() string {
	return e.approverType
}

func (e *hashKFilter) Remove(manager *manager.Manager) error {
	table, err := managerhelper.Map(manager, e.tableName)
	if err != nil {
		return err
	}
	return table.Delete(e.tableKey)
}

func (e *hashKFilter) Apply(manager *manager.Manager) error {
	table, err := managerhelper.Map(manager, e.tableName)
	if err != nil {
		return err
	}
	return table.Put(e.tableKey, e.value)
}
