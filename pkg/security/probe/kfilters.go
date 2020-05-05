package probe

import (
	"bytes"
	"encoding/binary"
	"fmt"

	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/golang-lru/simplelru"
)

type TableKey interface {
	Bytes() ([]byte, error)
	fmt.Stringer
}

type CommTableKey string

func (k CommTableKey) Bytes() ([]byte, error) {
	buffer := new(bytes.Buffer)
	if err := binary.Write(buffer, byteOrder, []byte(k)); err != nil {
		return nil, err
	}
	rep := make([]byte, 16)
	copy(rep, buffer.Bytes())
	return rep, nil
}

func (k CommTableKey) String() string {
	return string(k)
}

type KernelFilter struct {
	kind  string
	value []byte
}

func (kf *KernelFilter) Key() string {
	return kf.kind + "/" + string(kf.value)
}

type KernelFilters struct {
	tables map[string]eprobe.Table
	lru    *simplelru.LRU
}

func (kf *KernelFilters) getTable(kind string) (eprobe.Table, error) {
	table, found := kf.tables[kind]
	if !found {
		return nil, fmt.Errorf("unknown kernel filter '%s'", kind)
	}
	return table, nil
}

func (kf *KernelFilters) Push(kind string, value TableKey) error {
	log.Infof("Pushing kernel filter %s with value %s (%p)", kind, value, kf)

	table, err := kf.getTable(kind)
	if err != nil {
		return err
	}

	b, err := value.Bytes()
	if err != nil {
		return err
	}

	table.Set(b, []byte{1})
	filter := &KernelFilter{kind: kind, value: b}
	kf.lru.Add(filter.Key(), filter)

	return nil
}

func (kf *KernelFilters) removeFilter(kind string, value []byte) error {
	log.Infof("Removing filter %s with value %s\n", kind, string(value))

	table, err := kf.getTable(kind)
	if err != nil {
		return err
	}

	return table.Delete(value)
}

func (kf *KernelFilters) Pop(kind string, value TableKey) error {
	b, err := value.Bytes()
	if err != nil {
		return err
	}

	filter := KernelFilter{kind: kind, value: b}
	if !kf.lru.Remove(filter.Key()) {
		return fmt.Errorf("filter '%s' with value '%s' was not found", kind, value)
	}
	return nil
}

func NewKernelFilters(size int, kinds []string, probe *eprobe.Probe) (*KernelFilters, error) {
	kf := &KernelFilters{
		tables: make(map[string]eprobe.Table),
	}

	lru, err := simplelru.NewLRU(size, func(key interface{}, value interface{}) {
		filter := value.(*KernelFilter)
		kf.removeFilter(filter.kind, filter.value)
	})
	if err != nil {
		return nil, err
	}
	kf.lru = lru

	for _, kind := range kinds {
		table := probe.Table(kind)
		if table == nil {
			return nil, fmt.Errorf("failed to find table '%s'", kind)
		}
		kf.tables[kind] = table
	}

	return kf, nil
}
