package probe

import (
	"bytes"
	"encoding/binary"
)

type PolicyMode uint8
type PolicyFlag uint8

const (
	POLICY_MODE_ACCEPT PolicyMode = 1
	POLICY_MODE_DENY   PolicyMode = 2

	BASENAME_FLAG    PolicyFlag = 1
	FLAGS_FLAG       PolicyFlag = 2
	MODE_FLAG        PolicyFlag = 4
	PARENT_NAME_FLAG PolicyFlag = 8
	PROCESS_INODE    PolicyFlag = 16

	// need to be aligned with the kernel size
	BASENAME_FILTER_SIZE = 32
)

type KFilter interface {
	Bytes() []byte
}

type FilterPolicy struct {
	Mode  PolicyMode
	Flags PolicyFlag
}

func (f *FilterPolicy) Bytes() []byte {
	return []byte{uint8(f.Mode), uint8(f.Flags)}
}

type Uint8KFilter struct {
	value uint8
}

func (k *Uint8KFilter) Bytes() []byte {
	return []byte{k.value}
}

type Uint32KFilter struct {
	value uint32
}

func (k *Uint32KFilter) Bytes() []byte {
	b := make([]byte, 4)
	byteOrder.PutUint32(b, k.value)
	return b
}

type Uint64KFilter struct {
	value uint64
}

func (k *Uint64KFilter) Bytes() []byte {
	b := make([]byte, 8)
	byteOrder.PutUint64(b, k.value)
	return b
}

func StringToKey(str string, size int) ([]byte, error) {
	n := size
	if len(str) < size {
		n = len(str)
	}

	buffer := new(bytes.Buffer)
	if err := binary.Write(buffer, byteOrder, []byte(str)[0:n]); err != nil {
		return nil, err
	}
	rep := make([]byte, size)
	copy(rep, buffer.Bytes())
	return rep, nil
}

func Int32ToKey(i int32) ([]byte, error) {
	b := make([]byte, 4)
	byteOrder.PutUint32(b, uint32(i))
	return b
}

func Int64ToKey(i int64) ([]byte, error) {
	b := make([]byte, 8)
	byteOrder.PutUint64(b, uint64(i))
	return b
}
