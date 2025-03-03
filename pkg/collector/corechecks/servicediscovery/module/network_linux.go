// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package module

//go:generate mockgen -source=$GOFILE -package=$GOPACKAGE -destination=network_mock_linux.go

type networkCollector interface {
	close()
	addPid(pid uint32) error
	removePid(pid uint32) error
	getStats(pid uint32) (NetworkStats, error)
}
