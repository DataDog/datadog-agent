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

type activeKFilter interface {
	Remove(*manager.Manager) error
	Apply(*manager.Manager) error
	Key() interface{}
	GetTableName() string
	GetApproverType() string
}

// ActiveKFilters defines kfilter map
type ActiveKFilters map[interface{}]activeKFilter

func newActiveKFilters(kfilters ...activeKFilter) (ak ActiveKFilters) {
	ak = make(ActiveKFilters)
	for _, kfilter := range kfilters {
		if kfilter != nil {
			ak.Add(kfilter)
		}
	}
	return
}

// HasKey returns if a filter exists
func (ak ActiveKFilters) HasKey(key interface{}) bool {
	_, found := ak[key]
	return found
}

// Sub remove filters of the given filters
func (ak ActiveKFilters) Sub(ak2 ActiveKFilters) {
	for key := range ak {
		if _, found := ak2[key]; found {
			delete(ak, key)
		}
	}
}

// Add a filter
func (ak ActiveKFilters) Add(a activeKFilter) {
	ak[a.Key()] = a
}

// Remove a filter
func (ak ActiveKFilters) Remove(a activeKFilter) {
	delete(ak, a.Key())
}

type entryKey struct {
	tableName string
	key       interface{}
}

func makeEntryKey(tableName string, tableKey interface{}) entryKey {
	mb, ok := tableKey.(encoding.BinaryMarshaler)
	if !ok {
		return entryKey{
			tableName: tableName,
			key:       tableKey,
		}
	}

	data, _ := mb.MarshalBinary()

	return entryKey{
		tableName: tableName,
		key:       hex.EncodeToString(data),
	}
}

type arrayEntry struct {
	approverType string
	tableName    string
	index        interface{}
	value        interface{}
	zeroValue    interface{}
}

func (e *arrayEntry) Key() interface{} {
	return makeEntryKey(e.tableName, e.index)
}

func (e *arrayEntry) GetTableName() string {
	return e.tableName
}

func (e *arrayEntry) GetApproverType() string {
	return e.approverType
}

func (e *arrayEntry) Remove(manager *manager.Manager) error {
	table, err := managerhelper.Map(manager, e.tableName)
	if err != nil {
		return err
	}
	return table.Put(e.index, e.zeroValue)
}

func (e *arrayEntry) Apply(manager *manager.Manager) error {
	table, err := managerhelper.Map(manager, e.tableName)
	if err != nil {
		return err
	}
	return table.Put(e.index, e.value)
}

type eventMaskEntry struct {
	approverType string
	tableName    string
	tableKey     interface{}
	eventMask    uint64
}

func (e *eventMaskEntry) Key() interface{} {
	return makeEntryKey(e.tableName, e.tableKey)
}

func (e *eventMaskEntry) GetTableName() string {
	return e.tableName
}

func (e *eventMaskEntry) GetApproverType() string {
	return e.approverType
}

func (e *eventMaskEntry) Remove(manager *manager.Manager) error {
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

func (e *eventMaskEntry) Apply(manager *manager.Manager) error {
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

type hashEntry struct {
	approverType string
	tableName    string
	tableKey     interface{}
	value        interface{}
}

func (e *hashEntry) Key() interface{} {
	return makeEntryKey(e.tableName, e.tableKey)
}

func (e *hashEntry) GetTableName() string {
	return e.tableName
}

func (e *hashEntry) GetApproverType() string {
	return e.approverType
}

func (e *hashEntry) Remove(manager *manager.Manager) error {
	table, err := managerhelper.Map(manager, e.tableName)
	if err != nil {
		return err
	}
	return table.Delete(e.tableKey)
}

func (e *hashEntry) Apply(manager *manager.Manager) error {
	table, err := managerhelper.Map(manager, e.tableName)
	if err != nil {
		return err
	}
	return table.Put(e.tableKey, e.value)
}
