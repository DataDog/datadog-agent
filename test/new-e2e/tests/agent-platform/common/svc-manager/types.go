// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package svcmanager

// ServiceManager generic interface
type ServiceManager interface {
	Status(service string) (string, error)
	Start(service string) (string, error)
	Stop(service string) (string, error)
	Restart(service string) (string, error)
}
