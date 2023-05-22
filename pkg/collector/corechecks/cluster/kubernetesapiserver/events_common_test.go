// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
)

func TestGetDDAlertType(t *testing.T) {
	tests := []struct {
		name    string
		k8sType string
		want    metrics.EventAlertType
	}{
		{
			name:    "normal",
			k8sType: "Normal",
			want:    metrics.EventAlertTypeInfo,
		},
		{
			name:    "warning",
			k8sType: "Warning",
			want:    metrics.EventAlertTypeWarning,
		},
		{
			name:    "unknown",
			k8sType: "Unknown",
			want:    metrics.EventAlertTypeInfo,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDDAlertType(tt.k8sType)
			assert.Equal(t, got, tt.want)
		})
	}
}

func Test_getEventHostInfoImpl(t *testing.T) {
	providerIDFunc := func(clusterName string) string { return fmt.Sprintf("foo-%s", clusterName) }

	type args struct {
		clusterName string
		ev          *v1.Event
	}
	tests := []struct {
		name string
		args args
		want eventHostInfo
	}{
		{
			// the Kubelet source is providing the Host in the source section
			name: "node event from kubelet",
			args: args{
				clusterName: "my-cluster",
				ev: &v1.Event{
					InvolvedObject: v1.ObjectReference{
						Name: "my-node-1",
						Kind: nodeKind,
					},
					Source: v1.EventSource{
						Component: "kubelet",
						Host:      "my-node-1",
					},
				},
			},
			want: eventHostInfo{
				hostname: "my-node-1-my-cluster",
				nodename: "my-node-1",
			},
		},
		{
			// other controller like `draino`, `cluster-autoscaler` doesn't set the host name in source.
			name: "node event from kubelet",
			args: args{
				clusterName: "my-cluster",
				ev: &v1.Event{
					InvolvedObject: v1.ObjectReference{
						Name: "my-node-1",
						Kind: nodeKind,
					},
					Source: v1.EventSource{
						Component: "draino",
					},
				},
			},
			want: eventHostInfo{
				hostname: "my-node-1-my-cluster",
				nodename: "my-node-1",
			},
		},
		{
			// the Kubelet source is providing the Host in the source section
			name: "Pod event from kubelet",
			args: args{
				clusterName: "my-cluster",
				ev: &v1.Event{
					InvolvedObject: v1.ObjectReference{
						Name:      "my-pod-cdasd-adffd",
						Namespace: "foo",
						Kind:      podKind,
					},
					Source: v1.EventSource{
						Component: "kubelet",
						Host:      "my-node-1",
					},
				},
			},
			want: eventHostInfo{
				hostname: "my-node-1-my-cluster",
				nodename: "my-node-1",
			},
		},
		{
			// other controller like draino don't set the host in the source section
			// for now the Nodename will be empty, but with workload meta in the cluster-agent
			// we should be able to retrieve the Node name.
			name: "Pod event from draino",
			args: args{
				clusterName: "my-cluster",
				ev: &v1.Event{
					InvolvedObject: v1.ObjectReference{
						Name:      "my-pod-cdasd-adffd",
						Namespace: "foo",
						Kind:      podKind,
					},
					Source: v1.EventSource{
						Component: "draino",
					},
				},
			},
			want: eventHostInfo{
				hostname: "",
				nodename: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getEventHostInfoImpl(providerIDFunc, tt.args.clusterName, tt.args.ev); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getEventHostInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}
