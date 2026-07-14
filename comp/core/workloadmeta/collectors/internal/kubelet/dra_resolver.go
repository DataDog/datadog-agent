// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubelet && kubeapiserver

package kubelet

import (
	"context"
	"sync/atomic"

	resourcev1 "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	resourceinformersv1 "k8s.io/client-go/informers/resource/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	kubeletutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const nvidiaDRADriverName = "gpu.nvidia.com"

type draResourceResolver struct {
	informer cache.SharedIndexInformer
	synced   atomic.Bool
}

func (c *collector) installDRAResourceResolver(ctx context.Context) {
	if !c.config.GetBool("gpu.enabled") || !env.IsFeaturePresent(env.PodResources) {
		return
	}

	resolver, err := newDRAResourceResolver(ctx, c.kubeUtil)
	if err != nil {
		log.Debugf("DRA ResourceSlice resolver disabled: %s", err)
		return
	}

	resolverInstaller, ok := c.kubeUtil.(interface {
		SetDRAResourceResolver(kubeletutil.DRAResourceResolver)
	})
	if !ok {
		log.Debug("DRA ResourceSlice resolver disabled: kubelet utility does not accept a resolver")
		return
	}
	resolverInstaller.SetDRAResourceResolver(resolver)
}

func newDRAResourceResolver(ctx context.Context, kubeUtil kubeletutil.KubeUtilInterface) (*draResourceResolver, error) {
	nodeName, err := kubeUtil.GetNodename(ctx)
	if err != nil {
		return nil, err
	}

	apiClient, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, err
	}

	fieldSelector := fields.AndSelectors(
		fields.OneTermEqualSelector(resourcev1.ResourceSliceSelectorNodeName, nodeName),
		fields.OneTermEqualSelector(resourcev1.ResourceSliceSelectorDriver, nvidiaDRADriverName),
	).String()
	informer := resourceinformersv1.NewFilteredResourceSliceInformer(
		apiClient.InformerCl,
		0,
		cache.Indexers{},
		func(options *metav1.ListOptions) {
			options.FieldSelector = fieldSelector
		},
	)

	resolver := &draResourceResolver{informer: informer}
	go resolver.run(ctx)
	return resolver, nil
}

func (r *draResourceResolver) run(ctx context.Context) {
	go r.informer.Run(ctx.Done())
	if cache.WaitForCacheSync(ctx.Done(), r.informer.HasSynced) {
		r.synced.Store(true)
		return
	}
	log.Debug("DRA ResourceSlice resolver stopped before initial sync")
}

func (r *draResourceResolver) ResolveDRAResource(resource kubeletutil.ContainerClaimResource) (kubeletutil.ContainerAllocatedResource, bool) {
	if !r.synced.Load() || resource.DriverName != nvidiaDRADriverName {
		return kubeletutil.ContainerAllocatedResource{}, false
	}

	slices := make([]*resourcev1.ResourceSlice, 0, len(r.informer.GetStore().List()))
	for _, item := range r.informer.GetStore().List() {
		slice, ok := item.(*resourcev1.ResourceSlice)
		if !ok {
			continue
		}
		slices = append(slices, slice)
	}

	return resolveDRAResourceFromSlices(resource, slices)
}

func resolveDRAResourceFromSlices(resource kubeletutil.ContainerClaimResource, slices []*resourcev1.ResourceSlice) (kubeletutil.ContainerAllocatedResource, bool) {
	poolSlices := highestCompletePoolGeneration(resource, slices)
	if len(poolSlices) == 0 {
		return kubeletutil.ContainerAllocatedResource{}, false
	}

	for _, slice := range poolSlices {
		for _, device := range slice.Spec.Devices {
			if device.Name != resource.DeviceName {
				continue
			}
			uuid, ok := deviceAttributeString(device.Attributes, "uuid")
			if !ok {
				return kubeletutil.ContainerAllocatedResource{}, false
			}
			return kubeletutil.ContainerAllocatedResource{
				Name: nvidiaDRADriverName,
				ID:   uuid,
			}, true
		}
	}

	return kubeletutil.ContainerAllocatedResource{}, false
}

func highestCompletePoolGeneration(resource kubeletutil.ContainerClaimResource, slices []*resourcev1.ResourceSlice) []*resourcev1.ResourceSlice {
	var highestGeneration int64
	var generationSet bool
	for _, slice := range slices {
		if slice.Spec.Driver != resource.DriverName || slice.Spec.Pool.Name != resource.PoolName {
			continue
		}
		if !generationSet || slice.Spec.Pool.Generation > highestGeneration {
			highestGeneration = slice.Spec.Pool.Generation
			generationSet = true
		}
	}
	if !generationSet {
		return nil
	}

	var poolSlices []*resourcev1.ResourceSlice
	var expectedCount int64
	for _, slice := range slices {
		if slice.Spec.Driver != resource.DriverName ||
			slice.Spec.Pool.Name != resource.PoolName ||
			slice.Spec.Pool.Generation != highestGeneration {
			continue
		}
		if expectedCount == 0 {
			expectedCount = slice.Spec.Pool.ResourceSliceCount
		}
		if slice.Spec.Pool.ResourceSliceCount != expectedCount {
			return nil
		}
		poolSlices = append(poolSlices, slice)
	}

	if expectedCount == 0 || int64(len(poolSlices)) != expectedCount {
		return nil
	}
	return poolSlices
}

func deviceAttributeString(attributes map[resourcev1.QualifiedName]resourcev1.DeviceAttribute, name resourcev1.QualifiedName) (string, bool) {
	attribute, ok := attributes[name]
	if !ok || attribute.StringValue == nil {
		return "", false
	}
	return *attribute.StringValue, true
}
