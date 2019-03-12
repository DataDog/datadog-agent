// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"time"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
)

func (ku *KubeUtil) decodeAndFilterPodList(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	p := (*PodList)(ptr)
	cutoffTime := time.Now().Add(-1 * ku.podExpirationDuration)

	podCallback := func(iter *jsoniter.Iterator) bool {
		pod := &Pod{}
		iter.ReadVal(pod)

		// Quick exit for running/pending containers
		if pod.Status.Phase == "Running" || pod.Status.Phase == "Pending" {
			p.Items = append(p.Items, pod)
			return true
		}

		expired := true
		for _, ctr := range pod.Status.Containers {
			if ctr.State.Terminated == nil ||
				ctr.State.Terminated.FinishedAt.IsZero() ||
				ctr.State.Terminated.FinishedAt.After(cutoffTime) {
				expired = false
				break
			}
		}

		if !expired {
			p.Items = append(p.Items, pod)
		}
		return true
	}

	iter.ReadObjectCB(func(iter *jsoniter.Iterator, field string) bool {
		if field == "items" {
			iter.ReadArrayCB(podCallback)
		} else {
			iter.Skip()
		}
		return true
	})
}
