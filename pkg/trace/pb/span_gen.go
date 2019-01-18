package pb

import (
	"github.com/tinylib/msgp/msgp"
)

// DecodeMsg implements msgp.Decodable
func (z *Span) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zajw uint32
	zajw, err = dc.ReadMapHeader()
	if err != nil {
		return
	}
	for zajw > 0 {
		zajw--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			return
		}

		switch msgp.UnsafeString(field) {
		case "service":
			if dc.IsNil() {
				z.Service, err = "", dc.ReadNil()
				break
			}

			z.Service, err = parseString(dc)
			if err != nil {
				return
			}
		case "name":
			if dc.IsNil() {
				z.Name, err = "", dc.ReadNil()
				break
			}

			z.Name, err = parseString(dc)
			if err != nil {
				return
			}
		case "resource":
			if dc.IsNil() {
				z.Resource, err = "", dc.ReadNil()
				break
			}

			z.Resource, err = parseString(dc)
			if err != nil {
				return
			}
		case "trace_id":
			if dc.IsNil() {
				z.TraceID, err = 0, dc.ReadNil()
				break
			}

			z.TraceID, err = parseUint64(dc)
			if err != nil {
				return
			}
		case "span_id":
			if dc.IsNil() {
				z.SpanID, err = 0, dc.ReadNil()
				break
			}

			z.SpanID, err = parseUint64(dc)
			if err != nil {
				return
			}
		case "start":
			if dc.IsNil() {
				z.Start, err = 0, dc.ReadNil()
				break
			}

			z.Start, err = parseInt64(dc)
			if err != nil {
				return
			}
		case "duration":
			if dc.IsNil() {
				z.Duration, err = 0, dc.ReadNil()
				break
			}

			z.Duration, err = parseInt64(dc)
			if err != nil {
				return
			}
		case "error":
			if dc.IsNil() {
				z.Error, err = 0, dc.ReadNil()
				break
			}

			z.Error, err = parseInt32(dc)
			if err != nil {
				return
			}
		case "meta":
			if dc.IsNil() {
				z.Meta, err = nil, dc.ReadNil()
				break
			}

			var zwht uint32
			zwht, err = dc.ReadMapHeader()
			if err != nil {
				return
			}
			if z.Meta == nil && zwht > 0 {
				z.Meta = make(map[string]string, zwht)
			} else if len(z.Meta) > 0 {
				for key, _ := range z.Meta {
					delete(z.Meta, key)
				}
			}
			for zwht > 0 {
				zwht--
				var zxvk string
				var zbzg string
				zxvk, err = parseString(dc)
				if err != nil {
					return
				}
				zbzg, err = parseString(dc)
				if err != nil {
					return
				}
				z.Meta[zxvk] = zbzg
			}
		case "metrics":
			if dc.IsNil() {
				z.Metrics, err = nil, dc.ReadNil()
				break
			}

			var zhct uint32
			zhct, err = dc.ReadMapHeader()
			if err != nil {
				return
			}
			if z.Metrics == nil && zhct > 0 {
				z.Metrics = make(map[string]float64, zhct)
			} else if len(z.Metrics) > 0 {
				for key, _ := range z.Metrics {
					delete(z.Metrics, key)
				}
			}
			for zhct > 0 {
				zhct--
				var zbai string
				var zcmr float64
				zbai, err = parseString(dc)
				if err != nil {
					return
				}
				zcmr, err = parseFloat64(dc)
				if err != nil {
					return
				}
				z.Metrics[zbai] = zcmr
			}
		case "parent_id":
			if dc.IsNil() {
				z.ParentID, err = 0, dc.ReadNil()
				break
			}

			z.ParentID, err = parseUint64(dc)
			if err != nil {
				return
			}
		case "type":
			if dc.IsNil() {
				z.Type, err = "", dc.ReadNil()
				break
			}

			z.Type, err = parseString(dc)
			if err != nil {
				return
			}
		default:
			err = dc.Skip()
			if err != nil {
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z *Span) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 12
	// write "service"
	err = en.Append(0x8c, 0xa7, 0x73, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65)
	if err != nil {
		return err
	}
	err = en.WriteString(z.Service)
	if err != nil {
		return
	}
	// write "name"
	err = en.Append(0xa4, 0x6e, 0x61, 0x6d, 0x65)
	if err != nil {
		return err
	}
	err = en.WriteString(z.Name)
	if err != nil {
		return
	}
	// write "resource"
	err = en.Append(0xa8, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65)
	if err != nil {
		return err
	}
	err = en.WriteString(z.Resource)
	if err != nil {
		return
	}
	// write "trace_id"
	err = en.Append(0xa8, 0x74, 0x72, 0x61, 0x63, 0x65, 0x5f, 0x69, 0x64)
	if err != nil {
		return err
	}
	err = en.WriteUint64(z.TraceID)
	if err != nil {
		return
	}
	// write "span_id"
	err = en.Append(0xa7, 0x73, 0x70, 0x61, 0x6e, 0x5f, 0x69, 0x64)
	if err != nil {
		return err
	}
	err = en.WriteUint64(z.SpanID)
	if err != nil {
		return
	}
	// write "start"
	err = en.Append(0xa5, 0x73, 0x74, 0x61, 0x72, 0x74)
	if err != nil {
		return err
	}
	err = en.WriteInt64(z.Start)
	if err != nil {
		return
	}
	// write "duration"
	err = en.Append(0xa8, 0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e)
	if err != nil {
		return err
	}
	err = en.WriteInt64(z.Duration)
	if err != nil {
		return
	}
	// write "error"
	err = en.Append(0xa5, 0x65, 0x72, 0x72, 0x6f, 0x72)
	if err != nil {
		return err
	}
	err = en.WriteInt32(z.Error)
	if err != nil {
		return
	}
	// write "meta"
	err = en.Append(0xa4, 0x6d, 0x65, 0x74, 0x61)
	if err != nil {
		return err
	}
	err = en.WriteMapHeader(uint32(len(z.Meta)))
	if err != nil {
		return
	}
	for zxvk, zbzg := range z.Meta {
		err = en.WriteString(zxvk)
		if err != nil {
			return
		}
		err = en.WriteString(zbzg)
		if err != nil {
			return
		}
	}
	// write "metrics"
	err = en.Append(0xa7, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x73)
	if err != nil {
		return err
	}
	err = en.WriteMapHeader(uint32(len(z.Metrics)))
	if err != nil {
		return
	}
	for zbai, zcmr := range z.Metrics {
		err = en.WriteString(zbai)
		if err != nil {
			return
		}
		err = en.WriteFloat64(zcmr)
		if err != nil {
			return
		}
	}
	// write "parent_id"
	err = en.Append(0xa9, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x5f, 0x69, 0x64)
	if err != nil {
		return err
	}
	err = en.WriteUint64(z.ParentID)
	if err != nil {
		return
	}
	// write "type"
	err = en.Append(0xa4, 0x74, 0x79, 0x70, 0x65)
	if err != nil {
		return err
	}
	err = en.WriteString(z.Type)
	if err != nil {
		return
	}
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *Span) Msgsize() (s int) {
	s = 1 + 8 + msgp.StringPrefixSize + len(z.Service) + 5 + msgp.StringPrefixSize + len(z.Name) + 9 + msgp.StringPrefixSize + len(z.Resource) + 9 + msgp.Uint64Size + 8 + msgp.Uint64Size + 6 + msgp.Int64Size + 9 + msgp.Int64Size + 6 + msgp.Int32Size + 5 + msgp.MapHeaderSize
	if z.Meta != nil {
		for zxvk, zbzg := range z.Meta {
			_ = zbzg
			s += msgp.StringPrefixSize + len(zxvk) + msgp.StringPrefixSize + len(zbzg)
		}
	}
	s += 8 + msgp.MapHeaderSize
	if z.Metrics != nil {
		for zbai, zcmr := range z.Metrics {
			_ = zcmr
			s += msgp.StringPrefixSize + len(zbai) + msgp.Float64Size
		}
	}
	s += 10 + msgp.Uint64Size + 5 + msgp.StringPrefixSize + len(z.Type)
	return
}
