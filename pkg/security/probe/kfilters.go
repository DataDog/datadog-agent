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

	BASENAME_FLAG PolicyFlag = 1
	FLAGS_FLAG    PolicyFlag = 2

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

type Uint8Filter struct {
	value uint8
}

func (k *Uint8Filter) Bytes() []byte {
	return []byte{k.value}
}

type Uint32Filter struct {
	value uint32
}

func (k *Uint32Filter) Bytes() []byte {
	b := make([]byte, 4)
	byteOrder.PutUint32(b, k.value)
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
	buffer := new(bytes.Buffer)
	if err := binary.Write(buffer, byteOrder, i); err != nil {
		return nil, err
	}
	rep := make([]byte, binary.MaxVarintLen32)
	copy(rep, buffer.Bytes())
	return rep, nil
}
