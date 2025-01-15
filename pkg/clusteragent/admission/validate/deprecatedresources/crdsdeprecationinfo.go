// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package deprecatedresources is an admission controller webhook use to detect the usage of deprecated APIGroup and APIVersions.
package deprecatedresources

import (
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

type objectInfoMapType struct {
	info  map[schema.GroupVersionKind]deprecationInfoType
	mutex sync.RWMutex
}

type objectInfoMapInterface interface {
	IsDeprecated(gvk schema.GroupVersionKind) deprecationInfoType
	Replace(crds map[schema.GroupVersionKind]deprecationInfoType)
}

func newObjectInfoMap(in map[schema.GroupVersionKind]deprecationInfoType) objectInfoMapInterface {
	infoMap := &objectInfoMapType{
		info: in,
	}

	if infoMap.info == nil {
		infoMap.info = make(map[schema.GroupVersionKind]deprecationInfoType)
	}
	return infoMap
}

func (o *objectInfoMapType) IsDeprecated(gvk schema.GroupVersionKind) deprecationInfoType {
	if info, found := o.get(gvk); found {
		return info
	}
	return deprecationInfoType{}
}

func (o *objectInfoMapType) Replace(in map[schema.GroupVersionKind]deprecationInfoType) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.info = in
}

func (o *objectInfoMapType) get(gvk schema.GroupVersionKind) (deprecationInfoType, bool) {
	o.mutex.RLock()
	defer o.mutex.RUnlock()
	info, ok := o.info[gvk]
	return info, ok
}
