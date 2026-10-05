// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package sidecar

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patchtest"
)

func EnsureSharedSocketVolume(pod *corev1.Pod) string {
	var name string
	_, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		var err error
		name, err = PlanSharedSocketVolume(s)
		return true, err
	})
	if err != nil {
		panic(err)
	}
	return name
}
func MountSocketIntoContainer(pod *corev1.Pod, container, volume, directory string) error {
	_, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		return true, PlanSocketMount(s, container, volume, directory)
	})
	return err
}
func EnsureSocketFSGroup(pod *corev1.Pod, gid int64) {
	_, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		return true, s.EnsureFSGroup(gid)
	})
	if err != nil {
		panic(err)
	}
}
