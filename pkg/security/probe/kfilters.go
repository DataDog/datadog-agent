// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ErrDiscarderNotSupported is returned when trying to discover a discarder on a field that doesn't support them
type ErrDiscarderNotSupported struct {
	Field string
}

func (e ErrDiscarderNotSupported) Error() string {
	return fmt.Sprintf("discarder not supported for `%s`", e.Field)
}

// FilterPolicy describes a filtering policy
type FilterPolicy struct {
	Mode  PolicyMode
	Flags PolicyFlag
}

// Bytes returns the binary representation of a FilterPolicy
func (f *FilterPolicy) Bytes() ([]byte, error) {
	return []byte{uint8(f.Mode), uint8(f.Flags)}, nil
}

// KFilterApplier implements the Applier interface and applies passing
// policy by setting a value in a single entry eBPF array
type KFilterApplier struct {
	reporter Applier
	probe    *Probe
}

func (k *KFilterApplier) setFilterPolicy(tableName string, mode PolicyMode, flags PolicyFlag) error {
	table := k.probe.Table(tableName)
	if table == nil {
		return fmt.Errorf("unable to find policy table `%s`", tableName)
	}

	policy := &FilterPolicy{
		Mode:  mode,
		Flags: flags,
	}

	return table.Set(ebpf.ZeroUint32TableItem, policy)
}

// ApplyFilterPolicy is called when a passing policy for an event type is applied
func (k *KFilterApplier) ApplyFilterPolicy(eventType eval.EventType, tableName string, mode PolicyMode, flags PolicyFlag) error {
	log.Infof("Setting in-kernel filter policy to `%s` for `%s`", mode, eventType)

	if err := k.reporter.ApplyFilterPolicy(eventType, tableName, mode, flags); err != nil {
		return err
	}

	return k.setFilterPolicy(tableName, mode, flags)
}

// ApplyApprovers applies approvers
func (k *KFilterApplier) ApplyApprovers(eventType eval.EventType, hookPoint *HookPoint, approvers rules.Approvers) error {
	if err := k.reporter.ApplyApprovers(eventType, hookPoint, approvers); err != nil {
		return err
	}

	return hookPoint.OnNewApprovers(k.probe, approvers)
}

// GetReport returns the report
func (k *KFilterApplier) GetReport() *Report {
	return k.reporter.GetReport()
}

func isParentPathDiscarder(rs *rules.RuleSet, eventType eval.EventType, filename string) (bool, error) {
	dirname := filepath.Dir(filename)

	// ensure we don't push parent discarder if there is another rule relying on the parent path
	// ex: rule      open.filename == "/etc/passwd"
	//     discarder /etc/fstab
	// /etc/fstab is a discarder but not the parent
	re, err := regexp.Compile("^" + dirname + "/.*$")
	if err != nil {
		return false, err
	}

	values := rs.GetFieldValues(eventType + ".filename")
	for _, value := range values {
		if re.MatchString(value.Value.(string)) {
			return false, nil
		}
	}

	// check basename, assuming there is a basename field
	// ensure that we don't discard a parent that matches a basename rule
	// ex: rule     open.basename == ".ssh"
	//     discader /root/.ssh/id_rsa
	// we can't discard /root/.ssh as basename rule matches it
	// Note: This shouldn't happen we can't have a discarder working on multiple fields.
	//       ex: open.filename == "/etc/passwd"
	//           open.basename == "shadow"
	//       These rules won't return any discarder
	isDiscarder, err := rs.IsDiscarder(eventType+".basename", path.Base(dirname))
	if err != nil {
		if _, ok := err.(*eval.ErrFieldNotFound); ok {
			// no basename rule so we can discard
			isDiscarder = true
		}
	}

	if isDiscarder {
		log.Debugf("`%s` discovered as parent discarder", dirname)
	}

	return isDiscarder, nil
}

func discardInode(probe *Probe, mountID uint32, inode uint64, tableName string) (bool, error) {
	key := pathKey{mountID: mountID, inode: inode}

	table := probe.Table(tableName)
	if err := table.Set(&key, ebpf.ZeroUint8TableItem); err != nil {
		return false, err
	}

	return true, nil
}

func discardParentInode(probe *Probe, rs *rules.RuleSet, eventType eval.EventType, filename string, mountID uint32, inode uint64, tableName string) (bool, error) {
	isDiscarder, err := isParentPathDiscarder(rs, eventType, filename)
	if !isDiscarder {
		return false, err
	}

	parentMountID, parentInode, err := probe.resolvers.DentryResolver.GetParent(mountID, inode)
	if err != nil {
		return false, err
	}

	return discardInode(probe, parentMountID, parentInode, tableName)
}

func approveBasename(probe *Probe, tableName string, basename string) error {
	key := ebpf.NewStringTableItem(basename, BasenameFilterSize)

	table := probe.Table(tableName)
	if err := table.Set(key, ebpf.ZeroUint8TableItem); err != nil {
		return err
	}

	return nil
}

func approveBasenames(probe *Probe, tableName string, basenames ...string) error {
	for _, basename := range basenames {
		if err := approveBasename(probe, tableName, basename); err != nil {
			return err
		}
	}
	return nil
}

func setFlagsFilter(probe *Probe, tableName string, flags ...int) error {
	var flagsItem ebpf.Uint32TableItem

	for _, flag := range flags {
		flagsItem |= ebpf.Uint32TableItem(flag)
	}

	if flagsItem != 0 {
		table := probe.Table(tableName)
		if err := table.Set(ebpf.ZeroUint32TableItem, flagsItem); err != nil {
			return err
		}
	}

	return nil
}

func approveFlags(probe *Probe, tableName string, flags ...int) error {
	return setFlagsFilter(probe, tableName, flags...)
}

func discardFlags(probe *Probe, tableName string, flags ...int) error {
	return setFlagsFilter(probe, tableName, flags...)
}

func approveProcessFilename(probe *Probe, tableName string, filename string) error {
	fileinfo, err := os.Stat(filename)
	if err != nil {
		return err
	}
	stat, _ := fileinfo.Sys().(*syscall.Stat_t)
	key := ebpf.Uint64TableItem(uint64(stat.Ino))

	table := probe.Table(tableName)
	if err := table.Set(key, ebpf.ZeroUint8TableItem); err != nil {
		return err
	}
	return nil
}

func approveProcessFilenames(probe *Probe, tableName string, filenames ...string) error {
	for _, filename := range filenames {
		if err := approveProcessFilename(probe, tableName, filename); err != nil {
			return err
		}
	}

	return nil
}
