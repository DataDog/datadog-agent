// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package model holds model related files
package model

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Commands:
// For the benchmarks:
// go test -run=^$ -bench='^Benchmark(DeepCopy|JSON)$' -benchmem ./pkg/security/secl/model
//
// For the tests:
// go test -v -run 'TestEvent_DeepCopy' ./pkg/security/secl/model

// TestMain prints benchmark column headers
func TestMain(m *testing.M) {
	// Check if we're running benchmarks
	for _, arg := range os.Args {
		if strings.Contains(arg, "-bench") || strings.Contains(arg, "bench=") {
			// Print benchmark headers
			fmt.Println("\n" + strings.Repeat("=", 80))
			fmt.Println("Event DeepCopy Benchmark Results")
			fmt.Println(strings.Repeat("=", 80))
			fmt.Println("\nColumn Descriptions:")
			fmt.Println("  • Iterations    - Number of times the benchmark was executed")
			fmt.Println("  • ns/op         - Nanoseconds per operation")
			fmt.Println("  • B/op          - Bytes allocated per operation")
			fmt.Println("  • allocs/op     - Memory allocations per operation")
			fmt.Println("\n" + strings.Repeat("-", 80))
			fmt.Println()
			break
		}
	}

	// Run tests/benchmarks
	exitCode := m.Run()
	os.Exit(exitCode)
}

// TestEvent_DeepCopy tests the deep copy functionality with all Event's direct fields
func TestEvent_DeepCopy(t *testing.T) {
	// Create event with ALL direct Event struct fields populated
	original := createFullyPopulatedEvent()

	// Perform deep copy
	copied := original.DeepCopy()

	// Verify copy is not nil
	require.NotNil(t, copied, "DeepCopy should return non-nil")

	// Serialize and compare
	originalJSON, err := json.Marshal(original)
	require.NoError(t, err, "Failed to marshal original")

	copiedJSON, err := json.Marshal(copied)
	require.NoError(t, err, "Failed to marshal copy")

	// JSON should be identical
	assert.JSONEq(t, string(originalJSON), string(copiedJSON), "Copy should have identical content")

}

// createFullyPopulatedEvent creates a fully populated event for benchmarking
func createFullyPopulatedEvent() *Event {
	e := &Event{}

	// BaseEvent fields
	e.ID = "test-event-123"
	e.Type = 42
	e.Flags = 100
	e.TimestampRaw = 1234567890
	e.Timestamp = time.Unix(1234567890, 0)
	e.Os = "linux"
	e.Origin = "runtime"
	e.Service = "my-service"
	e.Hostname = "test-host"
	e.RuleTags = []string{"security", "compliance", "audit"}
	e.Source = "runtime"

	// Rules
	e.Rules = []*MatchedRule{
		{
			RuleID:        "rule-001",
			RuleVersion:   "1.0",
			RuleTags:      map[string]string{"severity": "high", "category": "file"},
			PolicyName:    "test-policy",
			PolicyVersion: "2.0",
		},
	}

	// ProcessContext
	e.ProcessContext = &ProcessContext{
		Process: Process{
			PIDContext: PIDContext{
				Pid:       1234,
				Tid:       1235,
				NetNS:     12345,
				IsKworker: false,
			},
			FileEvent: FileEvent{
				PathnameStr: "/usr/bin/test",
				BasenameStr: "test",
				Filesystem:  "ext4",
				FileFields: FileFields{
					UID:   1000,
					User:  "testuser",
					GID:   1000,
					Group: "testgroup",
					Mode:  0755,
					CTime: 1234567800,
					MTime: 1234567900,
					PathKey: PathKey{
						Inode:   987654,
						MountID: 42,
					},
				},
			},
			CGroup: CGroupContext{
				CGroupID: "docker-abc123",
			},
			ContainerContext: ContainerContext{
				ContainerID: "container-123",
				CreatedAt:   1234567000,
				Tags:        []string{"env:prod", "app:web"},
			},
			TTYName: "pts/0",
			Comm:    "test",
			Credentials: Credentials{
				UID:          1000,
				GID:          1000,
				User:         "testuser",
				Group:        "testgroup",
				EUID:         1000,
				EGID:         1000,
				EUser:        "testuser",
				EGroup:       "testgroup",
				FSUID:        1000,
				FSGID:        1000,
				FSUser:       "testuser",
				FSGroup:      "testgroup",
				CapEffective: 0x1234,
				CapPermitted: 0x5678,
			},
			UserSession: UserSessionContext{
				ID:          "123",
				SessionType: 1,
				Identity:    "k8s-user",
				K8SSessionContext: K8SSessionContext{
					K8SSessionID: 123,
					K8SUsername:  "k8s-user",
					K8SUID:       "k8s-uid-123",
					K8SGroups:    []string{"system:masters", "developers"},
					K8SExtra: map[string][]string{
						"scopes":      {"read", "write"},
						"extraGroups": {"group1", "group2"},
					},
				},
			},
			Argv0:         "test",
			Args:          "-v --debug",
			Argv:          []string{"-v", "--debug"},
			ArgsTruncated: false,
			Envs:          []string{"PATH", "HOME", "USER"},
			Envp:          []string{"PATH=/usr/bin", "HOME=/home/test", "USER=testuser"},
			EnvsTruncated: false,
			IsThread:      false,
			IsExec:        true,
		},
	}

	// SecurityProfileContext
	e.SecurityProfileContext = SecurityProfileContext{
		Name:       "production-profile",
		Version:    "v1.2.3",
		Tags:       []string{"env:prod", "team:security"},
		EventTypes: []EventType{1, 2, 3, 4},
	}

	// SpanContext
	e.SpanContext = SpanContext{
		SpanID:  9876543210,
		TraceID: utils.TraceID{Lo: 0x0102030405060708, Hi: 0x090a0b0c0d0e0f10},
	}

	// NetworkContext
	e.NetworkContext = NetworkContext{
		Device: NetworkDeviceContext{
			NetNS:   12345,
			IfIndex: 2,
			IfName:  "eth0",
		},
		L3Protocol: 2048, // IPv4
		L4Protocol: 6,    // TCP
		Source: IPPortContext{
			IPNet:    net.IPNet{IP: net.ParseIP("192.168.1.100"), Mask: net.CIDRMask(24, 32)},
			Port:     54321,
			IsPublic: false,
		},
		Destination: IPPortContext{
			IPNet:    net.IPNet{IP: net.ParseIP("8.8.8.8"), Mask: net.CIDRMask(32, 32)},
			Port:     443,
			IsPublic: true,
		},
		NetworkDirection: 1,
		Size:             1500,
		Type:             1,
	}

	// Event.Async
	e.Async = true

	// File events - Chmod
	e.Chmod.SyscallEvent.Retval = 0
	e.Chmod.File.PathnameStr = "/etc/passwd"
	e.Chmod.File.Inode = 123456
	e.Chmod.File.BasenameStr = "passwd"
	e.Chmod.Mode = 0644

	e.Chown.File.PathnameStr = "/tmp/test"
	e.Chown.File.BasenameStr = "test"
	e.Chown.UID = 1000
	e.Chown.User = "newuser"
	e.Chown.GID = 1000
	e.Chown.Group = "newgroup"

	e.Open.File.PathnameStr = "/var/log/auth.log"
	e.Open.File.BasenameStr = "auth.log"
	e.Open.Flags = 0x8000
	e.Open.Mode = 0644

	e.Mkdir.File.PathnameStr = "/tmp/newdir"
	e.Mkdir.File.BasenameStr = "newdir"
	e.Mkdir.Mode = 0755

	e.Rmdir.File.PathnameStr = "/tmp/olddir"
	e.Rmdir.File.BasenameStr = "olddir"

	e.Rename.Old.PathnameStr = "/tmp/old"
	e.Rename.Old.BasenameStr = "old"
	e.Rename.New.PathnameStr = "/tmp/new"
	e.Rename.New.BasenameStr = "new"

	e.Unlink.File.PathnameStr = "/tmp/deleted"
	e.Unlink.File.BasenameStr = "deleted"
	e.Unlink.Flags = 0

	e.Utimes.File.PathnameStr = "/tmp/utimes"
	e.Utimes.File.BasenameStr = "utimes"
	e.Utimes.Atime = time.Unix(1234567800, 0)
	e.Utimes.Mtime = time.Unix(1234567900, 0)

	e.Link.Source.PathnameStr = "/tmp/source"
	e.Link.Source.BasenameStr = "source"
	e.Link.Target.PathnameStr = "/tmp/target"
	e.Link.Target.BasenameStr = "target"

	e.SetXAttr.File.PathnameStr = "/tmp/xattr"
	e.SetXAttr.File.BasenameStr = "xattr"
	e.SetXAttr.Namespace = "user"
	e.SetXAttr.Name = "test.attribute"

	e.RemoveXAttr.File.PathnameStr = "/tmp/rmxattr"
	e.RemoveXAttr.File.BasenameStr = "rmxattr"

	e.Splice.File.PathnameStr = "/tmp/splice"
	e.Splice.File.BasenameStr = "splice"
	e.Splice.PipeEntryFlag = 1
	e.Splice.PipeExitFlag = 2

	e.Mount.MountID = 100
	e.Mount.FSType = "ext4"
	e.Mount.MountPointPath = "/mnt/data"
	e.Mount.MountSourcePath = "/dev/sda1"
	e.Mount.MountRootPath = "/"

	e.Chdir.File.PathnameStr = "/home/user"
	e.Chdir.File.BasenameStr = "user"

	// Process events - Exec
	e.Exec.Process = &Process{
		PIDContext: PIDContext{Pid: 1000, Tid: 1001},
		Comm:       "bash",
		FileEvent: FileEvent{
			PathnameStr: "/bin/bash",
			BasenameStr: "bash",
		},
		Argv0:         "bash",
		Args:          "-c 'echo hello'",
		Argv:          []string{"-c", "echo hello"},
		ArgsTruncated: false,
	}
	e.Exec.FileMetadata = FileMetadata{
		Size:         1024000,
		Type:         1,
		IsExecutable: true,
		Architecture: 1,
		ABI:          1,
	}

	e.SetUID.UID = 0
	e.SetUID.User = "root"
	e.SetUID.EUID = 0
	e.SetUID.EUser = "root"
	e.SetUID.FSUID = 0
	e.SetUID.FSUser = "root"

	e.SetGID.GID = 0
	e.SetGID.Group = "root"
	e.SetGID.EGID = 0
	e.SetGID.EGroup = "root"
	e.SetGID.FSGID = 0
	e.SetGID.FSGroup = "root"

	e.Capset.CapEffective = 0x1234
	e.Capset.CapPermitted = 0x5678

	e.Signal.Type = 9 // SIGKILL
	e.Signal.PID = 1234

	e.Exit.Cause = 0
	e.Exit.Code = 0

	e.Setrlimit.Resource = 7
	e.Setrlimit.RlimCur = 1024
	e.Setrlimit.RlimMax = 2048

	e.CapabilitiesUsage.Attempted = 0xABCD
	e.CapabilitiesUsage.Used = 0xEF01

	e.PrCtl.Option = 15
	e.PrCtl.NewName = "newname"
	e.PrCtl.IsNameTruncated = false

	// Network syscalls
	e.Bind.Addr.IPNet = net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(0, 32)}
	e.Bind.Addr.Port = 8080
	e.Bind.AddrFamily = 2 // AF_INET
	e.Bind.Protocol = 6   // TCP

	e.Connect.Addr.IPNet = net.IPNet{IP: net.ParseIP("1.2.3.4"), Mask: net.CIDRMask(32, 32)}
	e.Connect.Addr.Port = 443
	e.Connect.Hostnames = []string{"example.com", "www.example.com"}
	e.Connect.AddrFamily = 2
	e.Connect.Protocol = 6

	e.Accept.Addr.IPNet = net.IPNet{IP: net.ParseIP("192.168.1.50"), Mask: net.CIDRMask(32, 32)}
	e.Accept.Addr.Port = 80
	e.Accept.Hostnames = []string{"client.local"}
	e.Accept.AddrFamily = 2

	e.SetSockOpt.SocketType = 1
	e.SetSockOpt.SocketFamily = 2
	e.SetSockOpt.FilterLen = 10
	e.SetSockOpt.Level = 1
	e.SetSockOpt.OptName = 1

	// Kernel events
	e.SELinux.BoolName = "httpd_can_network_connect"
	e.SELinux.BoolChangeValue = "on"
	e.SELinux.BoolCommitValue = true
	e.SELinux.EnforceStatus = "enforcing"

	e.BPF.Cmd = 5
	e.BPF.Map.ID = 123
	e.BPF.Map.Type = 1
	e.BPF.Map.Name = "test_map"
	e.BPF.Program.ID = 456
	e.BPF.Program.Type = 2
	e.BPF.Program.AttachType = 1
	e.BPF.Program.Helpers = []uint32{1, 2, 3, 5, 6}
	e.BPF.Program.Name = "test_prog"
	e.BPF.Program.Tag = "abc123def456"

	e.PTrace.Request = 1
	e.PTrace.PID = 9999
	e.PTrace.Address = 0x1234567890

	e.MMap.File.PathnameStr = "/lib/libc.so"
	e.MMap.File.BasenameStr = "libc.so"
	e.MMap.Addr = 0x7fff00000000
	e.MMap.Offset = 0
	e.MMap.Len = 1024000
	e.MMap.Protection = 5 // PROT_READ | PROT_EXEC
	e.MMap.Flags = 2      // MAP_PRIVATE

	e.MProtect.VMStart = 0x7fff00000000
	e.MProtect.VMEnd = 0x7fff00100000
	e.MProtect.VMProtection = 3
	e.MProtect.ReqProtection = 7

	e.LoadModule.Name = "test_module"
	e.LoadModule.File.PathnameStr = "/lib/modules/test.ko"
	e.LoadModule.File.BasenameStr = "test.ko"
	e.LoadModule.Args = "param1=value1 param2=value2"
	e.LoadModule.Argv = []string{"param1=value1", "param2=value2"}
	e.LoadModule.ArgsTruncated = false
	e.LoadModule.LoadedFromMemory = false

	e.UnloadModule.Name = "old_module"

	e.SysCtl.Name = "net.ipv4.ip_forward"
	e.SysCtl.Action = 1
	e.SysCtl.Value = "1"
	e.SysCtl.OldValue = "0"
	e.SysCtl.NameTruncated = false
	e.SysCtl.ValueTruncated = false
	e.SysCtl.OldValueTruncated = false
	e.SysCtl.FilePosition = 0

	e.CgroupWrite.File.PathnameStr = "/sys/fs/cgroup/test"
	e.CgroupWrite.File.BasenameStr = "test"
	e.CgroupWrite.Pid = 1234

	// Network events
	e.DNS.ID = 12345
	e.DNS.Question.Name = "example.com"
	e.DNS.Question.Type = 1  // A record
	e.DNS.Question.Class = 1 // IN
	e.DNS.Question.Size = 32
	e.DNS.Question.Count = 1
	e.DNS.Response = &DNSResponse{
		ResponseCode: 0, // NOERROR
	}

	e.IMDS.Type = "aws"
	e.IMDS.CloudProvider = "aws"
	e.IMDS.URL = "http://169.254.169.254/latest/meta-data/"
	e.IMDS.Host = "169.254.169.254"
	e.IMDS.UserAgent = "curl/7.68.0"
	e.IMDS.Server = "EC2ws"
	e.IMDS.AWS.IsIMDSv2 = true
	e.IMDS.AWS.SecurityCredentials.Type = "AWS-HMAC"
	e.IMDS.AWS.SecurityCredentials.Code = "Success"

	e.RawPacket.Device.IfName = "eth0"
	e.RawPacket.Device.NetNS = 12345
	e.RawPacket.Device.IfIndex = 2
	e.RawPacket.L3Protocol = 2048 // IPv4
	e.RawPacket.L4Protocol = 6    // TCP
	e.RawPacket.Source.Port = 12345
	e.RawPacket.Destination.Port = 80
	e.RawPacket.Size = 1500
	e.RawPacket.TLSContext.Version = 0x0303 // TLS 1.2
	e.RawPacket.Filter = "tcp and port 80"

	e.NetworkFlowMonitor.Device.IfName = "eth0"
	e.NetworkFlowMonitor.Device.NetNS = 12345
	e.NetworkFlowMonitor.Device.IfIndex = 2
	e.NetworkFlowMonitor.FlowsCount = 2
	e.NetworkFlowMonitor.Flows = []Flow{
		{
			Source: IPPortContext{
				IPNet: net.IPNet{IP: net.ParseIP("192.168.1.10"), Mask: net.CIDRMask(32, 32)},
				Port:  50000,
			},
			Destination: IPPortContext{
				IPNet: net.IPNet{IP: net.ParseIP("10.0.0.1"), Mask: net.CIDRMask(32, 32)},
				Port:  443,
			},
			L3Protocol: 2048,
			L4Protocol: 6,
			Ingress: NetworkStats{
				DataSize:    1024000,
				PacketCount: 100,
			},
			Egress: NetworkStats{
				DataSize:    2048000,
				PacketCount: 200,
			},
		},
		{
			Source: IPPortContext{
				IPNet: net.IPNet{IP: net.ParseIP("192.168.1.11"), Mask: net.CIDRMask(32, 32)},
				Port:  50001,
			},
			Destination: IPPortContext{
				IPNet: net.IPNet{IP: net.ParseIP("10.0.0.2"), Mask: net.CIDRMask(32, 32)},
				Port:  80,
			},
			L3Protocol: 2048,
			L4Protocol: 6,
			Ingress: NetworkStats{
				DataSize:    512000,
				PacketCount: 50,
			},
			Egress: NetworkStats{
				DataSize:    768000,
				PacketCount: 75,
			},
		},
	}

	e.FailedDNS.Payload = []byte{0x01, 0x02, 0x03, 0x04, 0x05}

	// On-demand
	e.OnDemand.ID = 999
	e.OnDemand.Name = "custom_probe"
	e.OnDemand.Arg1Str = "arg1"
	e.OnDemand.Arg1Uint = 100
	e.OnDemand.Arg2Str = "arg2"
	e.OnDemand.Arg2Uint = 200
	e.OnDemand.Arg3Str = "arg3"
	e.OnDemand.Arg3Uint = 300

	// Internal events (field:"-")
	e.Umount.MountID = 42

	e.InvalidateDentry.Inode = 888888
	e.InvalidateDentry.MountID = 99

	e.MountReleased.MountID = 88

	e.CgroupTracing.ContainerContext.ContainerID = "traced-container"
	e.CgroupTracing.ContainerContext.CreatedAt = 1234567000
	e.CgroupTracing.Pid = 5555

	e.NetDevice.Device.Name = "veth0"
	e.NetDevice.Device.NetNS = 12345
	e.NetDevice.Device.IfIndex = 10

	e.VethPair.HostDevice.Name = "veth-host"
	e.VethPair.HostDevice.NetNS = 1
	e.VethPair.HostDevice.IfIndex = 5
	e.VethPair.PeerDevice.Name = "veth-peer"
	e.VethPair.PeerDevice.NetNS = 12345
	e.VethPair.PeerDevice.IfIndex = 6

	e.UnshareMountNS.MountID = 77
	e.UnshareMountNS.FSType = "overlay"
	e.UnshareMountNS.Device = 999

	return e
}

// deepCopyViaJSON performs deep copy using JSON marshal/unmarshal
func deepCopyViaJSON(e *Event) *Event {
	data, _ := json.Marshal(e)
	var copied Event
	_ = json.Unmarshal(data, &copied)
	return &copied
}

// BenchmarkDeepCopy benchmarks our DeepCopy on a fully populated event
func BenchmarkDeepCopy(b *testing.B) {
	event := createFullyPopulatedEvent()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		copied := event.DeepCopy()
		_ = copied
	}
}

// BenchmarkJSON benchmarks JSON marshal/unmarshal on a fully populated event
func BenchmarkJSON(b *testing.B) {
	event := createFullyPopulatedEvent()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		copied := deepCopyViaJSON(event)
		_ = copied
	}
}
