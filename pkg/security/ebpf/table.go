// +build linux_bpf

package ebpf

import (
	"bytes"
	"encoding/binary"
	"unsafe"

	bpflib "github.com/iovisor/gobpf/elf"
)

// Table represents an eBPF map
type Table struct {
	*bpflib.Map
	module *bpflib.Module
}

// TableItem represents an eBPF map item, either a key or a vlue
type TableItem interface {
	Bytes() ([]byte, error)
}

// Get retrieves the value associated with a specified key
func (t *Table) Get(key []byte) ([]byte, error) {
	var value [1024]byte
	err := t.module.LookupElement(t.Map, unsafe.Pointer(&key[0]), unsafe.Pointer(&value[0]))
	return value[:], err
}

// Set associates a value with a key in an eBPF map
func (t *Table) Set(key, value TableItem) error {
	bKey, err := key.Bytes()
	if err != nil {
		return err
	}
	bValue, err := value.Bytes()
	if err != nil {
		return err
	}
	return t.module.UpdateElement(t.Map, unsafe.Pointer(&bKey[0]), unsafe.Pointer(&bValue[0]), 0)
}

// SetP associates a value with a key in an eBPF map
func (t *Table) SetP(key, value []byte) error {
	return t.module.UpdateElement(t.Map, unsafe.Pointer(&key[0]), unsafe.Pointer(&value[0]), 0)
}

// GetNext returns the key following the one passed as parameter
func (t *Table) GetNext(key []byte) (bool, []byte, []byte, error) {
	var value [1024]byte
	pKey := unsafe.Pointer(&key[0])
	nextKey := make([]byte, len(key))
	more, err := t.module.LookupNextElement(t.Map, pKey, unsafe.Pointer(&nextKey[0]), unsafe.Pointer(&value[0]))
	return more, nextKey, value[:], err
}

// Delete the entry associated with the specified key
func (t *Table) Delete(key []byte) error {
	return t.module.DeleteElement(t.Map, unsafe.Pointer(&key[0]))
}

// BytesTableItem describes a raw table key or value
type BytesTableItem []byte

// Bytes returns the binary representation of a BytesTableItem
func (i BytesTableItem) Bytes() ([]byte, error) {
	return []byte(i), nil
}

// Uint8TableItem describes an uint8 table key or value
type Uint8TableItem uint8

// Bytes returns the binary representation of a Uint8TableItem
func (i Uint8TableItem) Bytes() ([]byte, error) {
	return []byte{uint8(i)}, nil
}

// Uint32TableItem describes an uint32 table key or value
type Uint32TableItem uint32

// Bytes returns the binary representation of a Uint32TableItem
func (i Uint32TableItem) Bytes() ([]byte, error) {
	b := make([]byte, 4)
	ByteOrder.PutUint32(b, uint32(i))
	return b, nil
}

// Uint64TableItem describes an uint64 table key or value
type Uint64TableItem uint64

// Bytes returns the binary representation of a Uint64TableItem
func (i Uint64TableItem) Bytes() ([]byte, error) {
	b := make([]byte, 8)
	ByteOrder.PutUint64(b, uint64(i))
	return b, nil
}

// StringTableItem describes an string table key or value
type StringTableItem struct {
	str  string
	size int
}

// Bytes returns the binary representation of a StringTableItem
func (i *StringTableItem) Bytes() ([]byte, error) {
	n := i.size
	if len(i.str) < i.size {
		n = len(i.str)
	}

	buffer := new(bytes.Buffer)
	if err := binary.Write(buffer, ByteOrder, []byte(i.str)[0:n]); err != nil {
		return nil, err
	}
	rep := make([]byte, i.size)
	copy(rep, buffer.Bytes())
	return rep, nil
}

// NewStringTableItem returns a new StringTableItem
func NewStringTableItem(str string, size int) *StringTableItem {
	return &StringTableItem{str: str, size: size}
}

// Zero table items
var (
	ZeroUint8TableItem  = BytesTableItem([]byte{0})
	ZeroUint32TableItem = BytesTableItem([]byte{0, 0, 0, 0})
	ZeroUint64TableItem = BytesTableItem([]byte{0, 0, 0, 0, 0, 0, 0, 0})
)
