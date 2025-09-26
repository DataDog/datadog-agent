// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

type Dependencies = dependencies

func EraseActuator[A Actuator[AT], AT ActuatorTenant](a A) Actuator[ActuatorTenant] {
	return &erasedActuator[A, AT]{a: a}
}
