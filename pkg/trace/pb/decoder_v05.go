package pb

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/philhofer/fwd"
	"github.com/tinylib/msgp/msgp"
)

// dictionaryString reads an int from decoder dc and returns the string
// at that index from dict.
func dictionaryString(dc *msgp.Reader, dict []string) (string, error) {
	ui, err := dc.ReadUint32()
	if err != nil {
		return "", err
	}
	idx := int(ui)
	if idx >= len(dict) {
		return "", fmt.Errorf("dictionary index %d out of range", idx)
	}
	return dict[idx], nil
}

// DecodeMsgDictionary decodes a trace using the specification from the v0.5 endpoint.
// For details, see the documentation for endpoint v0.5 in pkg/trace/api/version.go
func (t *Traces) DecodeMsgDictionary(dc *msgp.Reader) error {
	if _, err := dc.ReadArrayHeader(); err != nil {
		return err
	}
	// read dictionary
	sz, err := dc.ReadArrayHeader()
	if err != nil {
		return err
	}
	dict := make([]string, sz)
	for i := range dict {
		str, err := parseString(dc)
		if err != nil {
			return err
		}
		dict[i] = str
	}
	// read traces
	sz, err = dc.ReadArrayHeader()
	if err != nil {
		return err
	}
	if cap(*t) >= int(sz) {
		*t = (*t)[:sz]
	} else {
		*t = make(Traces, sz)
	}
	for i := range *t {
		sz, err := dc.ReadArrayHeader()
		if err != nil {
			return err
		}
		if cap((*t)[i]) >= int(sz) {
			(*t)[i] = (*t)[i][:sz]
		} else {
			(*t)[i] = make(Trace, sz)
		}
		for j := range (*t)[i] {
			if (*t)[i][j] == nil {
				(*t)[i][j] = new(Span)
			}
			if err := (*t)[i][j].DecodeMsgDictionary(dc, dict); err != nil {
				return err
			}
		}
	}
	return nil
}

// spanPropertyCount specifies the number of top-level properties that a span
// has.
const spanPropertyCount = 12

// DecodeMsgDictionary decodes a span from the given decoder dc, looking up strings
// in the given dictionary dict. For details, see the documentation for endpoint v0.5
// in pkg/trace/api/version.go
func (z *Span) DecodeMsgDictionary(dc *msgp.Reader, dict []string) error {
	sz, err := dc.ReadArrayHeader()
	if err != nil {
		return err
	}
	if sz != spanPropertyCount {
		return errors.New("encoded span needs exactly 12 elements in array")
	}
	// Service (0)
	z.Service, err = dictionaryString(dc, dict)
	if err != nil {
		return err
	}
	// Name (1)
	z.Name, err = dictionaryString(dc, dict)
	if err != nil {
		return err
	}
	// Resource (2)
	z.Resource, err = dictionaryString(dc, dict)
	if err != nil {
		return err
	}
	// TraceID (3)
	z.TraceID, err = parseUint64(dc)
	if err != nil {
		return err
	}
	// SpanID (4)
	z.SpanID, err = parseUint64(dc)
	if err != nil {
		return err
	}
	// ParentID (5)
	z.ParentID, err = parseUint64(dc)
	if err != nil {
		return err
	}
	// Start (6)
	z.Start, err = parseInt64(dc)
	if err != nil {
		return err
	}
	// Duration (7)
	z.Duration, err = parseInt64(dc)
	if err != nil {
		return err
	}
	// Error (8)
	z.Error, err = parseInt32(dc)
	if err != nil {
		return err
	}
	// Meta (9)
	sz, err = dc.ReadMapHeader()
	if err != nil {
		return err
	}
	if z.Meta == nil && sz > 0 {
		z.Meta = make(map[string]string, sz)
	} else if len(z.Meta) > 0 {
		for key := range z.Meta {
			delete(z.Meta, key)
		}
	}
	for sz > 0 {
		sz--
		key, err := dictionaryString(dc, dict)
		if err != nil {
			return err
		}
		val, err := dictionaryString(dc, dict)
		if err != nil {
			return err
		}
		z.Meta[key] = val
	}
	// Metrics (10)
	sz, err = dc.ReadMapHeader()
	if err != nil {
		return err
	}
	if z.Metrics == nil && sz > 0 {
		z.Metrics = make(map[string]float64, sz)
	} else if len(z.Metrics) > 0 {
		for key := range z.Metrics {
			delete(z.Metrics, key)
		}
	}
	for sz > 0 {
		sz--
		key, err := dictionaryString(dc, dict)
		if err != nil {
			return err
		}
		val, err := parseFloat64(dc)
		if err != nil {
			return err
		}
		z.Metrics[key] = val
	}
	// Type (11)
	z.Type, err = dictionaryString(dc, dict)
	if err != nil {
		return err
	}
	return nil
}

var readerPool = sync.Pool{New: func() interface{} { return &msgp.Reader{} }}

// NewMsgpReader returns a *msgp.Reader that
// reads from the provided reader. The
// reader will be buffered.
func NewMsgpReader(r io.Reader) *msgp.Reader {
	p := readerPool.Get().(*msgp.Reader)
	if p.R == nil {
		p.R = fwd.NewReader(r)
	} else {
		p.R.Reset(r)
	}
	return p
}

// FreeMsgpReader marks reader r as done.
func FreeMsgpReader(r *msgp.Reader) { readerPool.Put(r) }
