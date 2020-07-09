package probe

import (
	"bytes"
	"encoding/binary"
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

func Int64ToKey(i int64) []byte {
	b := make([]byte, 8)
	byteOrder.PutUint64(b, uint64(i))
	return b
}

var zeroInt32 = []byte{0, 0, 0, 0}
var zeroInt8 = []byte{0}
