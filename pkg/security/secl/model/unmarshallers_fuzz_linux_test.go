// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds model related files
//
// Run all fuzz targets locally for 30s each:
//
//	for f in $(go test -list 'Fuzz.*' . | grep '^Fuzz'); do echo "=== $f ===" && go test -run '^$' -fuzz="^${f}\$" -fuzztime=30s . || break; done
package model

import (
	"testing"
)

// fuzzUnmarshaller is a helper that fuzzes any BinaryUnmarshaler implementation,
// verifying that it does not panic and that the returned read count is valid.
func fuzzUnmarshaller(f *testing.F, factory func() BinaryUnmarshaler, minSize int) {
	f.Helper()

	// Seed corpus: empty, too-short, exact minimum, and larger buffers
	f.Add([]byte{})
	if minSize > 0 {
		f.Add(make([]byte, minSize-1))
	}
	f.Add(make([]byte, minSize))
	f.Add(make([]byte, minSize+64))
	f.Add(make([]byte, 512))

	f.Fuzz(func(t *testing.T, data []byte) {
		e := factory()
		n, err := e.UnmarshalBinary(data)
		if err != nil {
			return
		}
		if n < 0 || n > len(data) {
			t.Fatalf("read count out of bounds: got %d for input length %d", n, len(data))
		}
	})
}

func FuzzEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &Event{} }, 16)
}

func FuzzChmodEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &ChmodEvent{} }, 88)
}

func FuzzChownEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &ChownEvent{} }, 96)
}

func FuzzSetuidEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SetuidEvent{} }, 16)
}

func FuzzSetgidEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SetgidEvent{} }, 16)
}

func FuzzCapsetEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &CapsetEvent{} }, 16)
}

func FuzzCredentials_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &Credentials{} }, 48)
}

func FuzzLoginUIDWriteEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &LoginUIDWriteEvent{} }, 4)
}

func FuzzPathKey_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &PathKey{} }, 16)
}

func FuzzFileFields_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &FileFields{} }, FileFieldsSize)
}

func FuzzFileEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &FileEvent{} }, FileFieldsSize)
}

func FuzzOpenEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &OpenEvent{} }, 96)
}

func FuzzMkdirEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &MkdirEvent{} }, 92)
}

func FuzzLinkEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &LinkEvent{} }, 160)
}

func FuzzRenameEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &RenameEvent{} }, 160)
}

func FuzzRmdirEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &RmdirEvent{} }, 88)
}

func FuzzUnlinkEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &UnlinkEvent{} }, 96)
}

func FuzzChdirEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &ChdirEvent{} }, 88)
}

func FuzzMount_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &Mount{} }, 88)
}

func FuzzMountEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &MountEvent{} }, 108)
}

func FuzzUnshareMountNSEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &UnshareMountNSEvent{} }, 88)
}

func FuzzUmountEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &UmountEvent{} }, 12)
}

func FuzzSetXAttrEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SetXAttrEvent{} }, 280)
}

func FuzzSELinuxEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SELinuxEvent{} }, 80)
}

func FuzzPIDContext_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &PIDContext{} }, 40)
}

func FuzzSyscallEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SyscallEvent{} }, 8)
}

func FuzzSyscallContext_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SyscallContext{} }, 8)
}

func FuzzSyscallsEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SyscallsEvent{} }, 72)
}

func FuzzSpanContext_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SpanContext{} }, 24)
}

func FuzzExitEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &ExitEvent{} }, 4)
}

func FuzzInvalidateDentryEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &InvalidateDentryEvent{} }, 16)
}

func FuzzArgsEnvsEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &ArgsEnvsEvent{} }, 12)
}

func FuzzProcess_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &Process{} }, 292)
}

func FuzzBPFEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &BPFEvent{} }, 100)
}

func FuzzBPFMap_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &BPFMap{} }, 24)
}

func FuzzBPFProgram_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &BPFProgram{} }, 64)
}

func FuzzPTraceEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &PTraceEvent{} }, 28)
}

func FuzzMMapEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &MMapEvent{} }, 120)
}

func FuzzMProtectEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &MProtectEvent{} }, 40)
}

func FuzzLoadModuleEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &LoadModuleEvent{} }, 268)
}

// panic: runtime error: slice bounds out of range [:272] with capacity 268 [recovered, repanicked]
func TestRegressionUnmarshalBinary(_ *testing.T) {
	e := &LoadModuleEvent{}
	data := make([]byte, 268)
	_, _ = e.UnmarshalBinary(data)
}

func FuzzUnloadModuleEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &UnloadModuleEvent{} }, 64)
}

func FuzzSignalEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SignalEvent{} }, 16)
}

func FuzzSpliceEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SpliceEvent{} }, 88)
}

func FuzzCgroupWriteEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &CgroupWriteEvent{} }, 24)
}

func FuzzNetworkDeviceContext_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &NetworkDeviceContext{} }, 8)
}

func FuzzNetworkContext_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &NetworkContext{} }, 56)
}

func FuzzDNSEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &DNSEvent{} }, 10)
}

func FuzzNetDevice_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &NetDevice{} }, 32)
}

func FuzzNetDeviceEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &NetDeviceEvent{} }, 40)
}

func FuzzVethPairEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &VethPairEvent{} }, 72)
}

func FuzzAcceptEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &AcceptEvent{} }, 28)
}

func FuzzBindEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &BindEvent{} }, 30)
}

func FuzzConnectEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &ConnectEvent{} }, 30)
}

func FuzzMountReleasedEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &MountReleasedEvent{} }, 16)
}

func FuzzCGroupContext_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &CGroupContext{} }, 16)
}

func FuzzUtimesEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &UtimesEvent{} }, 120)
}

func FuzzOnDemandEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &OnDemandEvent{} }, 4+OnDemandParsedArgsCount*OnDemandPerArgSize)
}

func FuzzNetworkStats_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &NetworkStats{} }, 16)
}

func FuzzFlow_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &Flow{} }, 72)
}

func FuzzNetworkFlowMonitorEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &NetworkFlowMonitorEvent{} }, 16)
}

func FuzzSysCtlEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SysCtlEvent{} }, 16)
}

func FuzzSetSockOptEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SetSockOptEvent{} }, 32)
}

func FuzzSetrlimitEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &SetrlimitEvent{} }, 32)
}

func FuzzCapabilitiesEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &CapabilitiesEvent{} }, 16)
}

func FuzzPrCtlEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &PrCtlEvent{} }, 20)
}

func FuzzTracerMemfdSealEvent_UnmarshalBinary(f *testing.F) {
	fuzzUnmarshaller(f, func() BinaryUnmarshaler { return &TracerMemfdSealEvent{} }, 12)
}
