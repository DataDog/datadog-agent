// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

//go:generate $GOPATH/bin/include_headers pkg/collector/corechecks/ebpf/c/runtime/oom-kill-kern.c pkg/ebpf/bytecode/build/runtime/oom-kill.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/oom-kill.c pkg/ebpf/bytecode/runtime/oom-kill.go runtime

// Package oomkill is the system-probe side of the OOM Kill check
package oomkill

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	bpflib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/oomkill/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/ebpf/features"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	oomMapName    = "oom_stats"
	oomKprobeName = "kprobe__oom_kill_process"
	oomFentryName = "fentry__oom_kill_process"
	oomHookName   = "oom_kill_process"
	// oomLinkName names the link pin, whichever program the link attaches
	oomLinkName = "oom_kill_process"
	moduleName  = "oom_kill"
	// oomStatsVersionField is the member of struct oom_stats whose type name, e.g. oom_stats_v1,
	// is bumped whenever the layout of the struct changes
	oomStatsVersionField = "oom_stats_version"
)

var supportsFentry = features.SupportsFentry

var (
	bpffsRoot = "/sys/fs/bpf"
	pinDir    = filepath.Join(bpffsRoot, "datadog-agent", "oom_kill")
)

// Probe is the eBPF side of the OOM Kill check
type Probe struct {
	coll *bpflib.Collection
	// progName is the program attached to oom_kill_process: oomFentryName or oomKprobeName
	progName string
	link     link.Link
	oomMap   *maps.GenericMap[uint64, oomStats]
	// pinned is true if this probe owns the bpffs pins of the link and map
	pinned bool
}

// NewProbe creates a [Probe].
//
// If cfg.PersistMonitoring is true, the probe is pinned to bpffs so that it keeps recording OOM kills
// after system-probe exits without a graceful shutdown. Whatever its value, a probe left pinned by
// a previous system-probe is taken over: the OOM kills it recorded are reported by the next
// [Probe.GetAndFlush], and when cfg.PersistMonitoring is false its pins are removed.
func NewProbe(cfg *Config) (*Probe, error) {
	if cfg.EnableCORE {
		probe, err := loadOOMKillCOREProbe(cfg)
		if err == nil {
			return probe, nil
		}

		if !cfg.AllowRuntimeCompiledFallback {
			return nil, fmt.Errorf("error loading CO-RE oom-kill probe: %s. set system_probe_config.allow_runtime_compiled_fallback to true to allow fallback to runtime compilation", err)
		}
		log.Warnf("error loading CO-RE oom-kill probe: %s. falling back to runtime compiled probe", err)
	}

	return loadOOMKillRuntimeCompiledProbe(cfg)
}

func loadOOMKillCOREProbe(cfg *Config) (*Probe, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("error detecting kernel version: %s", err)
	}
	if kv < kernel.VersionCode(4, 9, 0) {
		return nil, fmt.Errorf("detected kernel version %s, but oom-kill probe requires a kernel version of at least 4.9.0", kv)
	}

	var probe *Probe
	err = ebpf.LoadCOREAsset("oom-kill.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		probe, err = startOOMKillProbe(buf, opts.VerifierOptions, cfg)
		return err
	})
	if err != nil {
		return nil, err
	}

	log.Debugf("successfully loaded CO-RE version of oom-kill probe")
	return probe, nil
}

func loadOOMKillRuntimeCompiledProbe(cfg *Config) (*Probe, error) {
	buf, err := runtime.OomKill.Compile(&cfg.Config, getCFlags(&cfg.Config))
	if err != nil {
		return nil, err
	}
	defer buf.Close()

	return startOOMKillProbe(buf, bpflib.CollectionOptions{}, cfg)
}

func getCFlags(config *ebpf.Config) []string {
	cflags := []string{"-g"}
	if config.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}

func startOOMKillProbe(buf bytecode.AssetReader, collOpts bpflib.CollectionOptions, cfg *Config) (*Probe, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("failed to remove memlock rlimit: %w", err)
	}

	spec, err := bpflib.LoadCollectionSpecFromReader(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to load collection spec: %w", err)
	}
	mapSpec, ok := spec.Maps[oomMapName]
	if !ok {
		return nil, fmt.Errorf("map '%s' not found", oomMapName)
	}

	progSpec, err := selectProgram(spec, cfg.UseFentryPrograms)
	if err != nil {
		return nil, err
	}

	// Pins left by a previous system-probe are taken over and cleaned up below even when this one
	// does not pin, so that they are not left running forever.
	pinning := pinningFeaturesSupported(cfg, progSpec)

	// Reuse the map of the previous probe, so that the OOM kills recorded while no system-probe
	// was running are reported by the next check run.
	if m, err := bpflib.LoadPinnedMap(mapPinPath(oomMapName), nil); err == nil {
		// close the reference to the pinned map this program takes so it can be fully reaped
		// after the pin is removed
		defer m.Close()

		if err := compatibleMap(mapSpec, m); err != nil {
			log.Warnf("pinned oom-kill map is incompatible with the current probe, dropping its content: %s", err)
			_ = m.Close()
			removePin(mapPinPath(oomMapName))
		} else {
			if collOpts.MapReplacements == nil {
				collOpts.MapReplacements = make(map[string]*bpflib.Map)
			}
			collOpts.MapReplacements[oomMapName] = m
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Warnf("failed to load pinned oom-kill map, removing it: %s", err)
		removePin(mapPinPath(oomMapName))
	}

	coll, err := bpflib.NewCollectionWithOptions(spec, collOpts)
	if err != nil {
		var ve *bpflib.VerifierError
		if errors.As(err, &ve) {
			return nil, fmt.Errorf("verifier error loading oom-kill collection: %w\n%+v", err, ve)
		}
		return nil, fmt.Errorf("failed to load oom-kill collection: %w", err)
	}

	p := &Probe{coll: coll, progName: progSpec.Name}
	if p.oomMap, err = maps.Map[uint64, oomStats](coll.Maps[oomMapName]); err != nil {
		coll.Close()
		return nil, fmt.Errorf("failed to get map '%s': %w", oomMapName, err)
	}

	if err := p.attach(); err != nil {
		coll.Close()
		return nil, err
	}

	removePins()

	if pinning {
		if err := p.pin(); err != nil {
			log.Errorf("failed to pin ebpf resources: %v. Continuing unpinned.", err)
		}
	}

	ebpf.AddNameMappingsCollection(coll, moduleName)
	return p, nil
}

// compatibleMap checks that the pinned map can be reused by the current probe: on top of the
// attributes checked by MapSpec.Compatible, its values must have the same version marker, since a
// change of layout keeping the same size would otherwise be decoded wrongly.
func compatibleMap(mapSpec *bpflib.MapSpec, m *bpflib.Map) error {
	if err := mapSpec.Compatible(m); err != nil {
		return err
	}

	want, ok := btf.As[*btf.Struct](mapSpec.Value)
	if !ok {
		return fmt.Errorf("value of map '%s' is not a struct", oomMapName)
	}
	wantVersion, err := oomStatsVersion(want)
	if err != nil {
		return err
	}

	h, err := m.Handle()
	if err != nil {
		return fmt.Errorf("get BTF of pinned map: %w", err)
	}
	defer h.Close()
	pinnedSpec, err := h.Spec(nil)
	if err != nil {
		return fmt.Errorf("parse BTF of pinned map: %w", err)
	}
	var got *btf.Struct
	if err := pinnedSpec.TypeByName(want.Name, &got); err != nil {
		return fmt.Errorf("find struct %s in BTF of pinned map: %w", want.Name, err)
	}
	gotVersion, err := oomStatsVersion(got)
	if err != nil {
		return fmt.Errorf("pinned map: %w", err)
	}

	if gotVersion != wantVersion {
		return fmt.Errorf("pinned map has version %s, expected %s", gotVersion, wantVersion)
	}
	return nil
}

// oomStatsVersion returns the name of the type of the version marker of struct oom_stats
func oomStatsVersion(s *btf.Struct) (string, error) {
	for _, m := range s.Members {
		if m.Name != oomStatsVersionField {
			continue
		}
		td, ok := m.Type.(*btf.Typedef)
		if !ok {
			return "", fmt.Errorf("field %s of struct %s is %v, not a typedef", oomStatsVersionField, s.Name, m.Type)
		}
		return td.Name, nil
	}
	return "", fmt.Errorf("struct %s has no field %s", s.Name, oomStatsVersionField)
}

func (k *Probe) pin() error {
	if err := os.MkdirAll(pinDir, 0o700); err != nil {
		return fmt.Errorf("create pin directory %s: %w", pinDir, err)
	}

	if err := k.oomMap.Map().Pin(mapPinPath(oomMapName)); err != nil {
		return fmt.Errorf("pin map: %w", err)
	}

	// A link taken over from a pin still records that path, which makes Pin to the same path a
	// no-op even though removePins deleted the file. Clear it so that the link is pinned again.
	_ = k.link.Unpin()
	if err := k.link.Pin(linkPinPath(oomLinkName)); err != nil {
		removePin(mapPinPath(oomMapName))
		return fmt.Errorf("pin link: %w", err)
	}

	k.pinned = true

	return nil
}

func (k *Probe) unpin() {
	ownsPin := func() bool {
		pinned, err := link.LoadPinnedLink(linkPinPath(oomLinkName), nil)
		if err != nil {
			return false
		}
		defer pinned.Close()
		pinnedInfo, err := pinned.Info()
		if err != nil {
			return false
		}
		ownInfo, err := k.link.Info()
		if err != nil {
			return false
		}
		return pinnedInfo.ID == ownInfo.ID
	}

	if !ownsPin() {
		log.Warnf("oom-kill pins are owned by another probe, leaving them")
		return
	}
	removePins()
}

func (k *Probe) attach() error {
	var pinnedLink link.Link

	if l, err := link.LoadPinnedLink(linkPinPath(oomLinkName), nil); err == nil {
		pinnedLink = l
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Warnf("failed to load pinned oom-kill link, removing it: %s", err)
		removePin(linkPinPath(oomLinkName))
	}

	prog := k.coll.Programs[k.progName]
	if pinnedProg := samePinnedProgram(pinnedLink, prog, k.oomMap.Map()); pinnedProg != nil {
		log.Infof("taking ownership of pinned oom-kill probe")
		_ = prog.Close()
		k.coll.Programs[k.progName] = pinnedProg
		k.link = pinnedLink
		return nil
	}
	// give up ownership over pinned link so it can be fully released
	if pinnedLink != nil {
		_ = pinnedLink.Close()
	}

	var l link.Link
	var err error
	if k.progName == oomFentryName {
		l, err = link.AttachTracing(link.TracingOptions{Program: prog})
	} else {
		l, err = link.Kprobe(oomHookName, prog, nil)
	}
	if err != nil {
		return fmt.Errorf("failed to attach %s to %s: %w", k.progName, oomHookName, err)
	}
	k.link = l

	return nil
}

// selectProgram keeps the fentry program in spec if the kernel supports fentry on oom_kill_process,
// and the kprobe program otherwise.
// Both run the same logic; fentry is preferred because its link can be pinned from 5.7 instead of
// 5.15 for the kprobe's perf event link.
func selectProgram(spec *bpflib.CollectionSpec, useFentry bool) (*bpflib.ProgramSpec, error) {
	progName, unused := oomKprobeName, oomFentryName
	if err := supportsFentry(oomHookName); err == nil && useFentry {
		progName, unused = oomFentryName, oomKprobeName
	} else if err != nil {
		log.Debugf("fentry is not supported on %s, using kprobe: %s", oomHookName, err)
	} else {
		log.Debugf("fentry disallowed %s, using kprobe: %s", oomHookName, err)
	}

	if s, ok := spec.Programs[progName]; ok {
		delete(spec.Programs, unused)
		return s, nil
	}

	return nil, fmt.Errorf("program '%s' not found", progName)
}

// Close releases all associated resources.
//
// On a graceful stop the probe is unpinned and detached, so that it does not outlive the module.
// It only keeps running when system-probe dies without calling Close (e.g. when it is OOM killed).
func (k *Probe) Close() {
	ebpf.RemoveNameMappingsCollection(k.coll)
	if k.pinned {
		k.unpin()
	}
	if err := k.link.Close(); err != nil {
		log.Errorf("error detaching OOM Kill probe: %s", err)
	}
	k.coll.Close()
}

// GetAndFlush gets the stats
func (k *Probe) GetAndFlush() (results []model.OOMKillStats) {
	var allTimestamps []uint64
	var ts uint64
	var stat oomStats
	it := k.oomMap.Iterate()
	for it.Next(&ts, &stat) {
		if out, err := convertStats(stat); err != nil {
			log.Errorf("%v", err)
		} else {
			results = append(results, out)
		}
		allTimestamps = append(allTimestamps, ts)
	}

	if err := it.Err(); err != nil {
		log.Warnf("failed to iterate on OOM stats while flushing: %s", err)
	}

	for _, ts := range allTimestamps {
		if err := k.oomMap.Delete(&ts); err != nil {
			log.Warnf("failed to delete stat: %s", err)
		}
	}

	return results
}

// convertStats converts a map value according to the version of its layout. It returns an error for
// an unknown version, whose value cannot be decoded.
func convertStats(in oomStats) (model.OOMKillStats, error) {
	switch v := any(in.Oom_stats_version).(type) {
	case oomStatsV1:
		return convertStatsV1(oomStatsV1Layout(in)), nil
	default:
		return model.OOMKillStats{}, fmt.Errorf("unknown oom_stats version %T, dropping OOM kill of pid %d", v, in.Victim_pid)
	}
}

func convertStatsV1(in oomStatsV1Layout) (out model.OOMKillStats) {
	out.CgroupName = unix.ByteSliceToString(in.Cgroup_name[:])
	out.VictimPid = in.Victim_pid
	out.TriggerPid = in.Trigger_pid
	out.Score = in.Score
	out.ScoreAdj = in.Score_adj
	out.VictimComm = unix.ByteSliceToString(in.Victim_comm[:])
	out.TriggerComm = unix.ByteSliceToString(in.Trigger_comm[:])
	out.Pages = in.Pages
	out.MemCgOOM = in.Memcg_oom
	return
}

// pinningFeaturesSupported reports whether the probe should be pinned to bpffs. All of the following must hold:
//   - oom_kill.persist_monitoring is enabled;
//   - bpffs is mounted at bpffsRoot;
//   - the fentry program is in use, its link can be pinned on this kernel, and the info of a
//     pinned link can be read from its fd.
//
// The kprobe program is never pinned: its perf event link can only be pinned from 5.15
func pinningFeaturesSupported(cfg *Config, progSpec *bpflib.ProgramSpec) bool {
	if !cfg.PersistMonitoring {
		return false
	}
	if !bpffsAvailable() {
		log.Warnf("bpffs is not mounted at %s, oom-kill probe will not be pinned", bpffsRoot)
		return false
	}
	if progSpec.Type != bpflib.Tracing {
		log.Debugf("oom-kill probe is using a kprobe, which is not pinned")
		return false
	}
	if err := fentryPinningSupported(); err != nil {
		log.Warnf("pinning fentry links is not supported, oom-kill probe will not be pinned: %s", err)
		return false
	}
	return true
}

// fentryPinningSupported attaches a dummy fentry program to oom_kill_process, pins its link in a
// temporary bpffs directory and reads the info of the link through the pin, then removes everything.
func fentryPinningSupported() error {
	prog, err := bpflib.NewProgramWithOptions(&bpflib.ProgramSpec{
		Type:       bpflib.Tracing,
		AttachType: bpflib.AttachTraceFEntry,
		AttachTo:   oomHookName,
		Instructions: asm.Instructions{
			asm.LoadImm(asm.R0, 0, asm.DWord),
			asm.Return(),
		},
		License: "GPL",
	}, bpflib.ProgramOptions{LogDisabled: true})
	if err != nil {
		return fmt.Errorf("load test program: %w", err)
	}
	defer prog.Close()

	l, err := link.AttachTracing(link.TracingOptions{Program: prog})
	if err != nil {
		return fmt.Errorf("attach test program: %w", err)
	}
	defer l.Close()

	dir, err := os.MkdirTemp(bpffsRoot, "datadog-agent-oom-kill-pin-test-")
	if err != nil {
		return fmt.Errorf("create test pin directory: %w", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "link")
	if err := l.Pin(path); err != nil {
		return fmt.Errorf("pin test link: %w", err)
	}

	// The next system-probe opens the pinned link and reads its info from the fd to take it over,
	// and Close does the same to check that it owns the pin. Without link info (before 5.8), a
	// pinned probe could neither be taken over nor unpinned.
	pinned, err := link.LoadPinnedLink(path, nil)
	if err != nil {
		return fmt.Errorf("load pinned test link: %w", err)
	}
	defer pinned.Close()
	if _, err := pinned.Info(); err != nil {
		return fmt.Errorf("read info of pinned test link: %w", err)
	}
	return l.Unpin()
}

// bpffsAvailable reports whether bpffsRoot is a mounted bpf filesystem
func bpffsAvailable() bool {
	var st unix.Statfs_t
	if err := unix.Statfs(bpffsRoot, &st); err != nil {
		return false
	}
	return st.Type == unix.BPF_FS_MAGIC
}

// samePinnedProgram returns the program attached to the pinned link if it is identical to prog
// and writes to the pinned map, which is then reused. Returns nil otherwise.
func samePinnedProgram(pinned link.Link, prog *bpflib.Program, maps ...*bpflib.Map) *bpflib.Program {
	if pinned == nil {
		return nil
	}
	linkInfo, err := pinned.Info()
	if err != nil {
		return nil
	}
	pinnedProg, err := bpflib.NewProgramFromID(linkInfo.Program)
	if err != nil {
		return nil
	}

	pinnedInfo, err := pinnedProg.Info()
	if err == nil {
		var progInfo *bpflib.ProgramInfo
		progInfo, err = prog.Info()
		// The kernel computes the tag over the instructions with map references zeroed out
		if err == nil && pinnedInfo.Tag != "" && pinnedInfo.Tag == progInfo.Tag && usesSameMaps(pinnedInfo, maps) {
			return pinnedProg
		}
	}
	_ = pinnedProg.Close()
	return nil
}

// usesSameMaps reports whether the program described by info references m
func usesSameMaps(info *bpflib.ProgramInfo, maps []*bpflib.Map) bool {
	ids, ok := info.MapIDs()
	if !ok {
		return false
	}

	for _, m := range maps {
		mapInfo, err := m.Info()
		if err != nil {
			return false
		}
		mapID, ok := mapInfo.ID()
		if !ok {
			return false
		}

		if !slices.Contains(ids, mapID) {
			return false
		}
	}

	return true
}

// removePins removes the link and map pins, and the pin directory if it is empty
func removePins() {
	removePin(linkPinPath(oomLinkName))
	removePin(mapPinPath(oomMapName))
	if err := os.Remove(pinDir); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, unix.ENOTEMPTY) {
		log.Debugf("failed to remove pin directory %s: %s", pinDir, err)
	}
}

func removePin(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Warnf("failed to remove pin %s: %s", path, err)
	}
}

func linkPinPath(name string) string { return filepath.Join(pinDir, name+"_link") }
func mapPinPath(name string) string  { return filepath.Join(pinDir, name) }
