// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build docker

// Package v2 provides an ECS client for v2 of the API.
package v2

import "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v2/v2client"

type (
	Task           v2client.task
	Container      v2client.Container
	Network        v2client.Network
	Port           v2client.Port
	ContainerStats v2client.ContainerStats
	NetStatsMap    v2client.NetStatsMap
	CPUStats       v2client.CPUStats
	CPUUsage       v2client.CPUUsage
	MemStats       v2client.MemStats
	DetailedMem    v2client.DetailedMem
	IOStats        v2client.IOStats
	OPStat         v2client.OPStat
	NetStats       v2client.NetStats
)
