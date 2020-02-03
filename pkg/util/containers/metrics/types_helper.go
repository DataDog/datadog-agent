// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package metrics

// SumInterfaces sums stats from all interfaces into a single InterfaceNetStats
func (ns ContainerNetStats) SumInterfaces() *InterfaceNetStats {
	sum := &InterfaceNetStats{}
	for _, stat := range ns {
		sum.BytesSent += stat.BytesSent
		sum.BytesRcvd += stat.BytesRcvd
		sum.PacketsSent += stat.PacketsSent
		sum.PacketsRcvd += stat.PacketsRcvd
	}
	return sum
}
