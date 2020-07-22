// +build linux_bpf

package probe

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
