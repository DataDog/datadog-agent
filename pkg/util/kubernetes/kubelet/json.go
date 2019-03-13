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

	"github.com/DataDog/datadog-agent/pkg/config"
)

// jsoniterConfig mirrors jsoniter.ConfigFastest
var jsonConfig = jsoniter.Config{
	EscapeHTML:                    false,
	MarshalFloatWith6Digits:       true, // will lose precession
	ObjectFieldMustBeSimpleString: true, // do not unescape object field
}

type podUnmarshaller struct {
	jsonConfig            jsoniter.API
	podExpirationDuration time.Duration
	timeNowFunction       func() time.Time // Allows to mock time in tests
}

func newPodUnmarshaller() *podUnmarshaller {
	pu := &podUnmarshaller{
		podExpirationDuration: config.Datadog.GetDuration("kubernetes_pod_expiration_minutes") * time.Minute,
		timeNowFunction:       time.Now,
	}

	if pu.podExpirationDuration > 0 {
		jsoniter.RegisterTypeDecoderFunc("kubelet.PodList", pu.decodeAndFilterPodList)
	} else {
		// Force-unregister for unit tests to pick up the right state
		jsoniter.RegisterTypeDecoder("kubelet.PodList", nil)
	}

	// Build a new frozen config to invalidate type decoder cache
	pu.jsonConfig = jsonConfig.Froze()

	return pu
}

func (pu *podUnmarshaller) Unmarshal(data []byte, v interface{}) error {
	return pu.jsonConfig.Unmarshal(data, v)
}

func (pu *podUnmarshaller) decodeAndFilterPodList(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	p := (*PodList)(ptr)
	cutoffTime := pu.timeNowFunction().Add(-1 * pu.podExpirationDuration)

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
