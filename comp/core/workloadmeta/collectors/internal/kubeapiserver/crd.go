// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	"k8s.io/client-go/tools/cache"

	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type crdEventHandler struct {
	wlm    workloadmeta.Component
	parser kubernetesresourceparsers.ObjectParser
}

// setupCRDInformer sets up event handlers for the shared CRD informer
func setupCRDInformer(wlm workloadmeta.Component, informerFactory externalversions.SharedInformerFactory) (cache.ResourceEventHandlerRegistration, error) {
	informer := informerFactory.Apiextensions().V1().CustomResourceDefinitions().Informer()
	crdHandler := &crdEventHandler{
		wlm:    wlm,
		parser: kubernetesresourceparsers.NewCRDParser(),
	}

	handlerRegistration, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    crdHandler.OnAdd,
		UpdateFunc: crdHandler.OnUpdate,
		DeleteFunc: crdHandler.OnDelete,
	})

	if err != nil {
		return nil, err
	}
	return handlerRegistration, nil
}

func (c *crdEventHandler) OnAdd(obj interface{}) {
	crd, ok := obj.(*apiextensionsv1.CustomResourceDefinition)
	if !ok {
		log.Errorf("expected CustomResourceDefinition but got %T", obj)
		return
	}

	entity := c.parser.Parse(crd)
	event := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: entity,
	}

	c.wlm.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   event.Type,
			Source: workloadmeta.SourceKubeAPIServer,
			Entity: event.Entity,
		},
	})
}

func (c *crdEventHandler) OnUpdate(_, newObj interface{}) {
	newCRD, ok := newObj.(*apiextensionsv1.CustomResourceDefinition)
	if !ok {
		log.Errorf("expected CustomResourceDefinition but got %T", newObj)
		return
	}

	entity := c.parser.Parse(newCRD)
	event := workloadmeta.Event{
		Type:   workloadmeta.EventTypeSet,
		Entity: entity,
	}

	c.wlm.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   event.Type,
			Source: workloadmeta.SourceKubeAPIServer,
			Entity: event.Entity,
		},
	})
}

func (c *crdEventHandler) OnDelete(obj interface{}) {
	crd, ok := obj.(*apiextensionsv1.CustomResourceDefinition)
	if !ok {
		// Handle DeletedFinalStateUnknown
		deletedState, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			log.Errorf("expected CustomResourceDefinition or DeletedFinalStateUnknown but got %T", obj)
			return
		}
		crd, ok = deletedState.Obj.(*apiextensionsv1.CustomResourceDefinition)
		if !ok {
			log.Errorf("DeletedFinalStateUnknown contained unexpected object: %T", deletedState.Obj)
			return
		}
	}

	entity := c.parser.Parse(crd)
	event := workloadmeta.Event{
		Type:   workloadmeta.EventTypeUnset,
		Entity: entity,
	}

	c.wlm.Notify([]workloadmeta.CollectorEvent{
		{
			Type:   event.Type,
			Source: workloadmeta.SourceKubeAPIServer,
			Entity: event.Entity,
		},
	})
}
