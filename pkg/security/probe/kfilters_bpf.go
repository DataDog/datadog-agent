// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

type activeKFilter interface {
	Remove(*Probe) error
	Apply(*Probe) error
	Key() interface{}
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

type mapEntry struct {
	tableName string
	key       interface{}
	tableKey  interface{}
	value     interface{}
}

type mapHash struct {
	tableName string
	key       interface{}
}

func (e *mapEntry) Key() interface{} {
	return mapHash{
		tableName: e.tableName,
		key:       e.key,
	}
}

func (e *mapEntry) Remove(probe *Probe) error {
	table, err := probe.Map(e.tableName)
	if err != nil {
		return err
	}
	return table.Delete(e.tableKey)
}

func (e *mapEntry) Apply(probe *Probe) error {
	table, err := probe.Map(e.tableName)
	if err != nil {
		return err
	}
	return table.Put(e.tableKey, e.value)
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

func (e *arrayEntry) Remove(probe *Probe) error {
	table, err := probe.Map(e.tableName)
	if err != nil {
		return err
	}
	return table.Put(e.index, e.zeroValue)
}

func (e *arrayEntry) Apply(probe *Probe) error {
	table, err := probe.Map(e.tableName)
	if err != nil {
		return err
	}
	return table.Put(e.index, e.value)
}
