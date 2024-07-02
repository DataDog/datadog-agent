package pack

import "strconv"

const (
	offsetPrefix    = '^'
	offsetPrefixStr = string(offsetPrefix)
)

type StringPackerInterface interface {
	Pack(s string) string
}

type StringPacker struct {
	// TODO: add a way to limit the size here. And when we do so we'll want a kind of LRU.
	stringToOffset map[string]string
	offset         int64
}

type StringUnPacker struct {
	offsetToString map[string]string
	offset         int64
}

func NewStringPacker() *StringPacker {
	return &StringPacker{
		stringToOffset: make(map[string]string),
	}
}

func NewStringUnPacker() *StringUnPacker {
	return &StringUnPacker{
		offsetToString: make(map[string]string),
	}
}

func (p *StringPacker) Pack(s string) string {
	p.offset++

	if o, ok := p.stringToOffset[s]; ok {
		return o
	} else {
		p.stringToOffset[s] = offsetPrefixStr + strconv.FormatInt(p.offset-1, 16)
		return s
	}
}

func (u *StringUnPacker) UnPack(sOrOffset string) string {
	u.offset++

	if sOrOffset[0] == offsetPrefix {
		if m, ok := u.offsetToString[sOrOffset[1:]]; ok {
			return m
		} else {
			return sOrOffset
		}
	} else {
		u.offsetToString[strconv.FormatInt(u.offset-1, 16)] = sOrOffset
		return sOrOffset
	}
}

type NoopStringPacker struct{}

func (p *NoopStringPacker) Pack(s string) string {
	return s
}

var _ StringPackerInterface = &StringPacker{}
var _ StringPackerInterface = &NoopStringPacker{}
