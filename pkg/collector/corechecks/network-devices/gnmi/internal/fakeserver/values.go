// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fakeserver

import (
	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
)

// ScalarUint64 returns a protobuf TypedValue for an unsigned integer scalar.
func ScalarUint64(value uint64) *gnmipb.TypedValue {
	return &gnmipb.TypedValue{
		Value: &gnmipb.TypedValue_UintVal{
			UintVal: value,
		},
	}
}

// ScalarInt64 returns a protobuf TypedValue for a signed integer scalar.
func ScalarInt64(value int64) *gnmipb.TypedValue {
	return &gnmipb.TypedValue{
		Value: &gnmipb.TypedValue_IntVal{
			IntVal: value,
		},
	}
}

// ScalarString returns a protobuf TypedValue for a string scalar.
func ScalarString(value string) *gnmipb.TypedValue {
	return &gnmipb.TypedValue{
		Value: &gnmipb.TypedValue_StringVal{
			StringVal: value,
		},
	}
}

// InterfaceInOctetsUpdate returns an Update for a keyed interface counter path using TypedValue.
func InterfaceInOctetsUpdate(interfaceName string, octets uint64) *gnmipb.Update {
	return &gnmipb.Update{
		Path: &gnmipb.Path{
			Elem: []*gnmipb.PathElem{
				{Name: "openconfig"},
				{Name: "interfaces"},
				{Name: "interface", Key: map[string]string{"name": interfaceName}},
				{Name: "state"},
				{Name: "counters"},
				{Name: "in-octets"},
			},
		},
		Val: ScalarUint64(octets),
	}
}

// InterfaceOutOctetsUpdate returns an Update for outbound octets on a keyed interface path.
func InterfaceOutOctetsUpdate(interfaceName string, octets uint64) *gnmipb.Update {
	return &gnmipb.Update{
		Path: &gnmipb.Path{
			Elem: []*gnmipb.PathElem{
				{Name: "openconfig"},
				{Name: "interfaces"},
				{Name: "interface", Key: map[string]string{"name": interfaceName}},
				{Name: "state"},
				{Name: "counters"},
				{Name: "out-octets"},
			},
		},
		Val: ScalarUint64(octets),
	}
}
