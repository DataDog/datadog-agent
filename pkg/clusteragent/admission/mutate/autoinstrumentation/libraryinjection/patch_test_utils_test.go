// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection_test

import (
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch/v5"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patchtest"
)

func runProviderForTest(pod *corev1.Pod, fn func(*patch.PodSession) libraryinjection.MutationResult) libraryinjection.MutationResult {
	var result libraryinjection.MutationResult
	_, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		result = fn(s)
		return result.Status == libraryinjection.MutationStatusInjected, s.Err()
	})
	if err != nil {
		return libraryinjection.MutationResult{Status: libraryinjection.MutationStatusError, Err: err}
	}
	return result
}
func injectInjectorForTest(provider libraryinjection.LibraryInjectionProvider, pod *corev1.Pod, cfg libraryinjection.InjectorConfig) libraryinjection.MutationResult {
	return runProviderForTest(pod, func(s *patch.PodSession) libraryinjection.MutationResult { return provider.PlanInjector(s, cfg) })
}
func injectLibraryForTest(provider libraryinjection.LibraryInjectionProvider, pod *corev1.Pod, cfg libraryinjection.LibraryConfig) libraryinjection.MutationResult {
	return runProviderForTest(pod, func(s *patch.PodSession) libraryinjection.MutationResult { return provider.PlanLibrary(s, cfg) })
}
func injectAPMLibrariesForTest(pod *corev1.Pod, cfg libraryinjection.LibraryInjectionConfig) error {
	var outcome error
	_, err := patchtest.Run(pod, "", nil, func(s *patch.PodSession, _ string, _ dynamic.Interface) (bool, error) {
		outcome = libraryinjection.PlanAPMLibraries(s, cfg)
		return false, s.Err()
	})
	if err != nil {
		return err
	}
	return outcome
}

type testPodPatcher struct {
	real     *libraryinjection.PodPatcher
	session  *patch.PodSession
	original []byte
	pod      *corev1.Pod
}

func newPodPatcherForTest(pod *corev1.Pod, filter func(*corev1.Container) bool) *testPodPatcher {
	raw, err := json.Marshal(pod)
	if err != nil {
		panic(err)
	}
	s, err := patch.NewPodSession(raw)
	if err != nil {
		panic(err)
	}
	return &testPodPatcher{real: libraryinjection.NewPodPatcher(s, filter), session: s, original: raw, pod: pod}
}
func (p *testPodPatcher) apply(err error) {
	if err != nil {
		panic(err)
	}
	wire, err := p.session.JSONPatch()
	if err != nil {
		panic(err)
	}
	ops, err := jsonpatch.DecodePatch(wire)
	if err != nil {
		panic(err)
	}
	opts := jsonpatch.NewApplyOptions()
	opts.SupportNegativeIndices = false
	opts.AllowMissingPathOnRemove = false
	opts.EnsurePathExistsOnAdd = false
	out, err := ops.ApplyWithOptions(p.original, opts)
	if err != nil {
		panic(err)
	}
	*p.pod = corev1.Pod{}
	if err := json.Unmarshal(out, p.pod); err != nil {
		panic(err)
	}
}
func (p *testPodPatcher) AddVolume(v corev1.Volume)           { p.apply(p.real.AddVolume(v)) }
func (p *testPodPatcher) AddVolumeMount(v corev1.VolumeMount) { p.apply(p.real.AddVolumeMount(v)) }
func (p *testPodPatcher) AddVolumeMountWithTarget(v corev1.VolumeMount, name string) {
	p.apply(p.real.AddVolumeMountWithTarget(v, name))
}
func (p *testPodPatcher) AddInitContainer(v corev1.Container) { p.apply(p.real.AddInitContainer(v)) }
func (p *testPodPatcher) AddEnvVar(v corev1.EnvVar)           { p.apply(p.real.AddEnvVar(v)) }
func (p *testPodPatcher) AddEnvVarWithJoin(name, value, separator string) {
	p.apply(p.real.AddEnvVarWithJoin(name, value, separator))
}
