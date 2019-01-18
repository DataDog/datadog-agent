package pb

// NOTE: THIS FILE WAS PRODUCED BY THE
// MSGP CODE GENERATION TOOL (github.com/tinylib/msgp)
// DO NOT EDIT

import (
	"github.com/tinylib/msgp/msgp"
)

// DecodeMsg implements msgp.Decodable
func (z *Trace) DecodeMsg(dc *msgp.Reader) (err error) {
	var xsz uint32
	xsz, err = dc.ReadArrayHeader()
	if err != nil {
		return
	}
	if cap((*z)) >= int(xsz) {
		(*z) = (*z)[:xsz]
	} else {
		(*z) = make(Trace, xsz)
	}
	for bzg := range *z {
		if dc.IsNil() {
			err = dc.ReadNil()
			if err != nil {
				return
			}
			(*z)[bzg] = nil
		} else {
			if (*z)[bzg] == nil {
				(*z)[bzg] = new(Span)
			}
			err = (*z)[bzg].DecodeMsg(dc)
			if err != nil {
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z Trace) EncodeMsg(en *msgp.Writer) (err error) {
	err = en.WriteArrayHeader(uint32(len(z)))
	if err != nil {
		return
	}
	for bai := range z {
		if z[bai] == nil {
			err = en.WriteNil()
			if err != nil {
				return
			}
		} else {
			err = z[bai].EncodeMsg(en)
			if err != nil {
				return
			}
		}
	}
	return
}

func (z Trace) Msgsize() (s int) {
	s = msgp.ArrayHeaderSize
	for bai := range z {
		if z[bai] == nil {
			s += msgp.NilSize
		} else {
			s += z[bai].Msgsize()
		}
	}
	return
}

// DecodeMsg implements msgp.Decodable
func (z *Traces) DecodeMsg(dc *msgp.Reader) (err error) {
	var xsz uint32
	xsz, err = dc.ReadArrayHeader()
	if err != nil {
		return
	}
	if cap((*z)) >= int(xsz) {
		(*z) = (*z)[:xsz]
	} else {
		(*z) = make(Traces, xsz)
	}
	for wht := range *z {
		var xsz uint32
		xsz, err = dc.ReadArrayHeader()
		if err != nil {
			return
		}
		if cap((*z)[wht]) >= int(xsz) {
			(*z)[wht] = (*z)[wht][:xsz]
		} else {
			(*z)[wht] = make(Trace, xsz)
		}
		for hct := range (*z)[wht] {
			if dc.IsNil() {
				err = dc.ReadNil()
				if err != nil {
					return
				}
				(*z)[wht][hct] = nil
			} else {
				if (*z)[wht][hct] == nil {
					(*z)[wht][hct] = new(Span)
				}
				err = (*z)[wht][hct].DecodeMsg(dc)
				if err != nil {
					return
				}
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z Traces) EncodeMsg(en *msgp.Writer) (err error) {
	err = en.WriteArrayHeader(uint32(len(z)))
	if err != nil {
		return
	}
	for cua := range z {
		err = en.WriteArrayHeader(uint32(len(z[cua])))
		if err != nil {
			return
		}
		for xhx := range z[cua] {
			if z[cua][xhx] == nil {
				err = en.WriteNil()
				if err != nil {
					return
				}
			} else {
				err = z[cua][xhx].EncodeMsg(en)
				if err != nil {
					return
				}
			}
		}
	}
	return
}

func (z Traces) Msgsize() (s int) {
	s = msgp.ArrayHeaderSize
	for cua := range z {
		s += msgp.ArrayHeaderSize
		for xhx := range z[cua] {
			if z[cua][xhx] == nil {
				s += msgp.NilSize
			} else {
				s += z[cua][xhx].Msgsize()
			}
		}
	}
	return
}
