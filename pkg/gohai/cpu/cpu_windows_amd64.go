// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

package cpu

import (
	"encoding/binary"
	"syscall"
	"unsafe"
)

// SYSTEM_LOGICAL_PROCESSOR_INFORMATION_SIZE is the size of
// SYSTEM_LOGICAL_PROCESSOR_INFORMATION struct
//
//nolint:revive
const SYSTEM_LOGICAL_PROCESSOR_INFORMATION_SIZE = 32

//nolint:unused
func getSystemLogicalProcessorInformationSize() int {
	return SYSTEM_LOGICAL_PROCESSOR_INFORMATION_SIZE
}

//nolint:unused
func byteArrayToProcessorStruct(data []byte) (info SYSTEM_LOGICAL_PROCESSOR_INFORMATION) {
	info.ProcessorMask = uintptr(binary.LittleEndian.Uint64(data))
	info.Relationship = int(binary.LittleEndian.Uint64(data[8:]))
	copy(info.dataunion[0:16], data[16:32])
	return
}

func byteArrayToGroupAffinity(data []byte) (affinity GROUP_AFFINITY, consumed uint32, err error) {
	err = nil
	affinity.Mask = uintptr(binary.LittleEndian.Uint64(data))
	affinity.Group = binary.LittleEndian.Uint16(data[8:])
	// can skip the reserved, but count it
	consumed = 16
	return

}
func byteArrayToProcessorInformationExStruct(data []byte) (info SYSTEM_LOGICAL_PROCESSOR_INFORMATION_EX, consumed uint32, err error) {
	err = nil
	info.Relationship = int(binary.LittleEndian.Uint32(data))
	info.Size = binary.LittleEndian.Uint32(data[4:])

	consumed = 8
	return
}

func byteArrayToProcessorRelationshipStruct(data []byte) (proc PROCESSOR_RELATIONSHIP, groupMask []GROUP_AFFINITY, consumed uint32, err error) {
	err = nil
	proc.Flags = data[0]
	proc.EfficiencyClass = data[1]
	proc.GroupCount = uint16(binary.LittleEndian.Uint32(data[22:]))
	consumed = 24
	if proc.GroupCount != 0 {
		gm := make([]GROUP_AFFINITY, proc.GroupCount)

		for i := uint16(0); i < proc.GroupCount; i++ {
			var used uint32
			var ga GROUP_AFFINITY
			ga, used, err = byteArrayToGroupAffinity(data[consumed:])
			if err != nil {
				return
			}
			gm[i] = ga
			consumed += used
		}
		groupMask = gm
	}
	return
}

func byteArrayToNumaNode(data []byte) (numa NUMA_NODE_RELATIONSHIP, consumed uint32, err error) {
	numa.NodeNumber = binary.LittleEndian.Uint32(data)
	// skip 20 bytes of reserved
	consumed = 24
	aff, used, err := byteArrayToGroupAffinity(data[consumed:])
	numa.GroupMask = aff
	consumed += used
	return
}

func byteArrayToRelationCache(data []byte) (cache CACHE_RELATIONSHIP, consumed uint32, err error) {
	cache.Level = data[0]
	cache.Associativity = data[1]
	cache.LineSize = binary.LittleEndian.Uint16(data[2:])
	cache.CacheSize = binary.LittleEndian.Uint32(data[4:])
	cache.CacheType = int(binary.LittleEndian.Uint32(data[8:]))
	// skip 20 bytes
	consumed = 32
	ga, used, err := byteArrayToGroupAffinity(data[consumed:])
	cache.GroupMask = ga
	consumed += used
	return

}

func byteArrayToRelationGroup(data []byte) (group GROUP_RELATIONSHIP, gi []PROCESSOR_GROUP_INFO, consumed uint32, err error) {
	group.MaximumGroupCount = binary.LittleEndian.Uint16(data)
	group.ActiveGroupCount = binary.LittleEndian.Uint16(data[4:])
	consumed = 24
	if group.ActiveGroupCount > 0 {
		groups := make([]PROCESSOR_GROUP_INFO, group.ActiveGroupCount)
		for i := uint16(0); i < group.ActiveGroupCount; i++ {
			groups[i].MaximumProcessorCount = data[consumed]
			consumed++
			groups[i].ActiveProcessorCount = data[consumed]
			consumed++
			consumed += 38 // reserved
			groups[i].ActiveProcessorMask = uintptr(binary.LittleEndian.Uint64(data[consumed:]))
			consumed += 8
		}
	}
	return
}

func computeCoresAndProcessors() (CPU_INFO, error) {
	var cpuInfo CPU_INFO
	var mod = syscall.NewLazyDLL("kernel32.dll")
	var getProcInfo = mod.NewProc("GetLogicalProcessorInformationEx")
	var buflen uint32

	// first, figure out how much we need
	status, _, callErr := getProcInfo.Call(uintptr(0xFFFF), // all relationships.
		uintptr(0),
		uintptr(unsafe.Pointer(&buflen)))
	if status == 0 {
		if callErr != ERROR_INSUFFICIENT_BUFFER {
			// only error we're expecing here is insufficient buffer
			// anything else is a failure
			return cpuInfo, callErr
		}
	} else {
		// this shouldn't happen. Errno won't be set (because the function)
		// succeeded.  So just return something to indicate we've failed
		return cpuInfo, syscall.Errno(1)
	}
	buf := make([]byte, buflen)
	status, _, callErr = getProcInfo.Call(
		uintptr(0xFFFF), // still want all relationships
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&buflen)))
	if status == 0 {
		return cpuInfo, callErr
	}
	// walk through each of the buffers

	bufused := uint32(0)
	for bufused < buflen {
		info, used, decodeerr := byteArrayToProcessorInformationExStruct(buf[bufused:])
		if decodeerr != nil {
			return cpuInfo, decodeerr
		}
		bufused += used
		if info.Size == 0 {
			break
		}
		switch info.Relationship {
		case RelationProcessorCore:
			core, groupMask, used, decodeerr := byteArrayToProcessorRelationshipStruct(buf[bufused:])
			if decodeerr != nil {
				return cpuInfo, decodeerr
			}
			bufused += used
			cpuInfo.corecount++
			for j := uint16(0); j < core.GroupCount; j++ {
				cpuInfo.logicalcount += countBits(uint64(groupMask[j].Mask))
			}
		case RelationNumaNode:
			_, used, decodeerr := byteArrayToNumaNode(buf[bufused:])
			if decodeerr != nil {
				return cpuInfo, decodeerr
			}
			cpuInfo.numaNodeCount++
			bufused += used

		case RelationCache:
			cache, used, decodeerr := byteArrayToRelationCache(buf[bufused:])
			if decodeerr != nil {
				return cpuInfo, decodeerr
			}
			bufused += used
			switch cache.Level {
			case 1:
				cpuInfo.l1CacheSize = cache.CacheSize
			case 2:
				cpuInfo.l2CacheSize = cache.CacheSize
			case 3:
				cpuInfo.l3CacheSize = cache.CacheSize
			}
		case RelationProcessorPackage:
			_, _, used, decodeerr := byteArrayToProcessorRelationshipStruct(buf[bufused:])
			if decodeerr != nil {
				return cpuInfo, decodeerr
			}
			bufused += used
			cpuInfo.pkgcount++

		case RelationGroup:
			group, groupInfo, used, decodeerr := byteArrayToRelationGroup(buf[bufused:])
			if decodeerr != nil {
				return cpuInfo, decodeerr
			}
			bufused += used
			cpuInfo.relationGroups += int(group.MaximumGroupCount)
			for _, info := range groupInfo {
				cpuInfo.maxProcsInGroups += int(info.MaximumProcessorCount)
				cpuInfo.activeProcsInGroups += int(info.ActiveProcessorCount)
			}

		}
	}

	return cpuInfo, nil
}
