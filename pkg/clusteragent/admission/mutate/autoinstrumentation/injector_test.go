// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestInjectorOptions(t *testing.T) {
	i := newInjector(time.Now(), "registry", "1")
	require.Equal(t, "registry/apm-inject:1", i.image)
}

func TestInjectorLibRequirements(t *testing.T) {
	mutators := containerMutators{
		containerSecurityContext{
			&corev1.SecurityContext{
				AllowPrivilegeEscalation: pointer.Ptr(false),
			},
		},
	}
	i := newInjector(time.Now(), "registry", "1",
		injectorWithLibRequirementOptions(libRequirementOptions{initContainerMutators: mutators}),
	)

	opts := i.requirements().libRequirementOptions
	require.Equal(t, 1, len(opts.initContainerMutators))

	container := corev1.Container{}
	err := opts.initContainerMutators[0].mutateContainer(&container)
	require.NoError(t, err)

	require.Equal(t, &corev1.SecurityContext{
		AllowPrivilegeEscalation: pointer.Ptr(false),
	}, container.SecurityContext)
}
