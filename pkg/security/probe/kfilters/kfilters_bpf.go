// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package kfilters holds kfilters related files
package kfilters

import (
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	manager "github.com/DataDog/ebpf-manager"
)

type activeKFilter interface {
	Remove(*manager.Manager) error
	Apply(*manager.Manager) error
	Key() interface{}
	GetTableName() string
}

type activeKFilters map[interface{}]activeKFilter

func newActiveKFilters(kfilters ...activeKFilter) (ak activeKFilters) {
	ak = make(map[interface{}]activeKFilter)
	for _, kfilter := range kfilters {
		ak.Add(kfilter)
	}
	return
}

func (ak activeKFilters) HasKey(key interface{}) bool {
	_, found := ak[key]
	return found
}

func (ak activeKFilters) Sub(ak2 activeKFilters) {
	for key := range ak {
		if _, found := ak2[key]; found {
			delete(ak, key)
		}
	}
}

func (ak activeKFilters) Add(a activeKFilter) {
	ak[a.Key()] = a
}

func (ak activeKFilters) Remove(a activeKFilter) {
	delete(ak, a.Key())
}

type mapHash struct {
	tableName string
	key       interface{}
}

type arrayEntry struct {
	tableName string
	index     interface{}
	value     interface{}
	zeroValue interface{}
}

func (e *arrayEntry) Key() interface{} {
	return mapHash{
		tableName: e.tableName,
		key:       e.index,
	}
}

func (e *arrayEntry) GetTableName() string {
	return e.tableName
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

type mapEventMask struct {
	tableName string
	tableKey  interface{}
	key       interface{}
	eventMask uint64
}

func (e *mapEventMask) Key() interface{} {
	return mapHash{
		tableName: e.tableName,
		key:       e.key,
	}
}

func (e *mapEventMask) GetTableName() string {
	return e.tableName
}

func (e *mapEventMask) Remove(manager *manager.Manager) error {
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

func (e *mapEventMask) Apply(manager *manager.Manager) error {
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
