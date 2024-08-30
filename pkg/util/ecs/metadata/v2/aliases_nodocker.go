// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build !docker

// Package v2 provides an ECS client for v2 of the API.
package v2

import (
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2/v2client"
)

// Aliases for v2client types.go
type (
	// Task represents a task as returned by the ECS metadata API v2.
	Task v2client.Task
	// Container represents a container within a task.
	Container v2client.Container
	// Network represents the network of a container
	Network v2client.Network
	// Port represents the ports of a container
	Port v2client.Port
	// ContainerStats represents the statistics of a container as returned by
	// the ECS metadata API v2.
	ContainerStats v2client.ContainerStats
	// NetStatsMap represents a map of networks stats
	NetStatsMap v2client.NetStatsMap
	// CPUStats represents an ECS container CPU usage
	CPUStats v2client.CPUStats
	// CPUUsage represents the details of ECS container CPU usage
	CPUUsage v2client.CPUUsage
	// MemStats represents an ECS container memory usage
	MemStats v2client.MemStats
	// DetailedMem stores detailed stats about memory usage
	DetailedMem v2client.DetailedMem
	// IOStats represents an ECS container IO throughput
	IOStats v2client.IOStats
	// OPStat stores a value (amount of op or bytes) for a kind of operation and a specific block device.
	OPStat v2client.OPStat
	// NetStats represents an ECS container network usage
	NetStats v2client.NetStats
)

// Aliases for shared v2client/client_nodocker.go types
type (
	// Client is an interface for ECS metadata v2 API clients.
	Client v2client.Client
)

// Aliases for shared v2client/client_nodocker.go functions
var (
	// NewDefaultClient creates a new client for the default metadata v2 API endpoint.
	NewDefaultClient = v2client.NewDefaultClient
	GetTask          = v2client.GetTask
	GetTaskWithTags  = v2client.GetTaskWithTags
)
