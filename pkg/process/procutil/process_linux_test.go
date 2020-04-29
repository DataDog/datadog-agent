// +build linux

package process

import (
	"os"
	"testing"
	"time"
)

var sink map[int32]*Process
var sink2 map[int32]*Stats
var statSink *statInfo
var memoryInfoSink *MemoryInfoStat
var memoryInfoExSink *MemoryInfoExStat
var statusSink *statusInfo

func BenchmarkLinuxAllProcessesOnPostgresProcFS(b *testing.B) {
	var err error

	// Set procfs to testdata location, does not include:
	//  /fd  - fd count
	//  /exe - symlink
	//  /cwd - symlink
	os.Setenv("HOST_PROC", "resources/linux_postgres/proc")
	defer os.Unsetenv("HOST_PROC")

	probe := NewProcessProbe()
	defer probe.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if sink, err = probe.ProcessesByPID(); err != nil {
			b.Fatal("error encountered on iteration", i, err)
		}
	}
}

func BenchmarkLinuxAllProcessesOnLocalProcFS(b *testing.B) {
	var err error

	probe := NewProcessProbe()
	defer probe.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if sink, err = probe.ProcessesByPID(); err != nil {
			b.Fatal("error encountered on iteration", i, err)
		}
	}
}

func BenchmarkLinuxProcessesStatsForPIDsOnPostgresProcFS(b *testing.B) {
	var err error

	// Set procfs to testdata location, does not include:
	//  /fd  - fd count
	//  /exe - symlink
	//  /cwd - symlink
	os.Setenv("HOST_PROC", "resources/linux_postgres/proc")
	defer os.Unsetenv("HOST_PROC")

	probe := NewProcessProbe()
	defer probe.Close()

	procsByPID, err := probe.ProcessesByPID()
	if err != nil {
		b.Fatal("error encountered", err)
	}

	pids := make([]int32, len(procsByPID))
	for pid := range procsByPID {
		pids = append(pids, pid)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if sink2, err = probe.ProcessStatsForPIDs(pids); err != nil {
			b.Fatal("error encountered on iteration", i, err)
		}
	}
}

func BenchmarkLinuxProcessesStatsForPIDsOnLocalProcFS(b *testing.B) {
	var err error

	probe := NewProcessProbe()
	defer probe.Close()

	procsByPID, err := probe.ProcessesByPID()
	if err != nil {
		b.Fatal("error encountered", err)
	}

	pids := make([]int32, len(procsByPID))
	for pid := range procsByPID {
		pids = append(pids, pid)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if sink2, err = probe.ProcessStatsForPIDs(pids); err != nil {
			b.Fatal("error encountered on iteration", i, err)
		}
	}
}

func BenchmarkLinuxParseStats(b *testing.B) {
	probe := NewProcessProbe()
	defer probe.Close()

	now := time.Now()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		statSink = probe.parseStat("resources/linux_postgres/proc/26073", 26073, now) // Random Postgres process
	}
}

func BenchmarkLinuxParseStatm(b *testing.B) {
	probe := NewProcessProbe()
	defer probe.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		memoryInfoSink, memoryInfoExSink = probe.parseStatm("resources/linux_postgres/proc/26073") // Random Postgres process
	}
}

func BenchmarkLinuxParseStatus(b *testing.B) {
	probe := NewProcessProbe()
	defer probe.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		statusSink = probe.parseStatus("resources/linux_postgres/proc/26073") // Random Postgres process
	}
}

//func TestLinuxAllProcessesOnLocalProcFSOutput(t *testing.T) {
//	probe := NewProcessProbe()
//	defer probe.Close()
//
//	procsByPID, err := probe.ProcessesByPID()
//	if err != nil {
//		t.Fatal("error encountered", err)
//	}
//
//	for _, proc := range procsByPID {
//		fmt.Printf("%+v\n", proc.Pid)
//	}
//
//	t.Fatal()
//}
