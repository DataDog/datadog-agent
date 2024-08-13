// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package gpu

import (
	"time"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
)

type KtimeConverter struct {
	refKtimeNs uint64
	refEpochNs uint64
}

func NewKtimeConverter() *KtimeConverter {
	ktime, _ := ddebpf.NowNanoseconds()
	epoch := time.Now().UTC().UnixNano()
	return &KtimeConverter{
		refKtimeNs: uint64(ktime),
		refEpochNs: uint64(epoch),
	}
}

func (kc *KtimeConverter) KtimeToEpoch(ktimeNs uint64) uint64 {
	return kc.refEpochNs + (ktimeNs - kc.refKtimeNs)
}
