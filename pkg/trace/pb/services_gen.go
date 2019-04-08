package pb

// NOTE: THIS FILE WAS PRODUCED BY THE
// MSGP CODE GENERATION TOOL (github.com/tinylib/msgp)
// DO NOT EDIT

import "github.com/tinylib/msgp/msgp"

// DecodeMsg implements msgp.Decodable
func (z *ServicesMetadata) DecodeMsg(dc *msgp.Reader) (err error) {
	var zxhx uint32
	zxhx, err = dc.ReadMapHeader()
	if err != nil {
		return
	}
	if (*z) == nil && zxhx > 0 {
		(*z) = make(ServicesMetadata, zxhx)
	} else if len((*z)) > 0 {
		for key := range *z {
			delete((*z), key)
		}
	}
	for zxhx > 0 {
		zxhx--
		var zajw string
		var zwht map[string]string
		zajw, err = parseString(dc)
		if err != nil {
			return
		}
		var zlqf uint32
		zlqf, err = dc.ReadMapHeader()
		if err != nil {
			return
		}
		if zwht == nil && zlqf > 0 {
			zwht = make(map[string]string, zlqf)
		} else if len(zwht) > 0 {
			for key := range zwht {
				delete(zwht, key)
			}
		}
		for zlqf > 0 {
			zlqf--
			var zhct string
			var zcua string
			zhct, err = parseString(dc)
			if err != nil {
				return
			}
			zcua, err = parseString(dc)
			if err != nil {
				return
			}
			zwht[zhct] = zcua
		}
		(*z)[zajw] = zwht
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z ServicesMetadata) EncodeMsg(en *msgp.Writer) (err error) {
	err = en.WriteMapHeader(uint32(len(z)))
	if err != nil {
		return
	}
	for zdaf, zpks := range z {
		err = en.WriteString(zdaf)
		if err != nil {
			return
		}
		err = en.WriteMapHeader(uint32(len(zpks)))
		if err != nil {
			return
		}
		for zjfb, zcxo := range zpks {
			err = en.WriteString(zjfb)
			if err != nil {
				return
			}
			err = en.WriteString(zcxo)
			if err != nil {
				return
			}
		}
	}
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z ServicesMetadata) Msgsize() (s int) {
	s = msgp.MapHeaderSize
	if z != nil {
		for zdaf, zpks := range z {
			_ = zpks
			s += msgp.StringPrefixSize + len(zdaf) + msgp.MapHeaderSize
			if zpks != nil {
				for zjfb, zcxo := range zpks {
					_ = zcxo
					s += msgp.StringPrefixSize + len(zjfb) + msgp.StringPrefixSize + len(zcxo)
				}
			}
		}
	}
	return
}
