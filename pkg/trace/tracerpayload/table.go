package tracerpayload

type TableTraces struct {
	StringTable []string
	Spans       []TableSpan
}

// StringTable holds strings by index
// All usage assumes correct integer values
type StringTable struct {
	strings []*SharedString
	// Map of string to where it exists in `strings`
	stringIndexes map[string]uint32
}

func (st *StringTable) Get(i uint32) string {
	return st.strings[i].s
}

func (st *StringTable) GetIndex(s string) (uint32, bool) {
	i, ok := st.stringIndexes[s]
	return i, ok
}

// Update replaces the string at index i if the string is unshared,
// otherwise a new string is added and the new index is returned
func (st *StringTable) Update(i uint32, newS string) uint32 {
	oldS := st.strings[i]
	// More than one reference, add a new string
	if oldS.refs > 1 {
		return st.Add(newS)
	} else {
		st.strings[i].s = newS
		delete(st.stringIndexes, oldS.s)
		st.stringIndexes[newS] = i
		return i
	}
}

// Add appends s to the table if it's new otherwise increments the ref of the existing string
func (st *StringTable) Add(s string) uint32 {
	// If the string is already there use it
	if newSIdx, ok := st.stringIndexes[s]; ok {
		st.strings[newSIdx].refs += 1
		return newSIdx
	}
	// If not, make a new entry
	st.strings = append(st.strings, &SharedString{
		s:    s,
		refs: 1,
	})
	newIndex := uint32(len(st.strings) - 1)
	st.stringIndexes[s] = newIndex
	return newIndex
}

type SharedString struct {
	s    string
	refs int
}

type TableSpan struct {
	// string table is shared by whole trace
	stringTable StringTable
	service     uint32
	name        uint32
	resource    uint32
	traceID     uint64
	spanID      uint64
	parentID    uint64
	start       int64
	duration    int64
	error       int32
	meta        map[uint32]uint32
	metrics     map[uint32]float64
	typ         uint32
}

func (t *TableSpan) TraceID() uint64 {
	return t.traceID
}

func (t *TableSpan) SpanID() uint64 {
	return t.spanID
}

func (t *TableSpan) ParentID() uint64 {
	return t.parentID
}

func (t *TableSpan) SetParentID(pid uint64) {
	t.parentID = pid
}

func (t *TableSpan) Duration() int64 {
	return t.duration
}

func (t *TableSpan) SetDuration(d int64) {
	t.duration = d
}

func (t *TableSpan) Start() int64 {
	return t.start
}

func (t *TableSpan) SetStart(s int64) {
	t.start = s
}

func (t *TableSpan) Service() string {
	return t.stringTable.Get(t.service)
}

func (t *TableSpan) SetService(s string) {
	t.service = t.stringTable.Update(t.service, s)
}

func (t *TableSpan) Name() string {
	return t.stringTable.Get(t.name)
}

func (t *TableSpan) SetName(n string) {
	t.name = t.stringTable.Update(t.name, n)
}

func (t *TableSpan) Resource() string {
	return t.stringTable.Get(t.resource)
}

func (t *TableSpan) SetResource(r string) {
	t.resource = t.stringTable.Update(t.resource, r)
}

func (t *TableSpan) Type() string {
	return t.stringTable.Get(t.typ)
}

func (t *TableSpan) SetType(ty string) {
	t.typ = t.stringTable.Update(t.typ, ty)
}

func (t *TableSpan) Error() int32 {
	return t.error
}

func (t *TableSpan) Meta(k string) (string, bool) {
	kIdx, ok := t.stringTable.GetIndex(k)
	if !ok {
		return "", false
	}
	vIdx, ok := t.meta[kIdx]
	if !ok {
		return "", false
	}
	return t.stringTable.Get(vIdx), true
}

func (t *TableSpan) ForMeta(f func(string, string) (string, string, bool)) bool {
	modified := false
	for kIdx, vIdx := range t.meta {
		k := t.stringTable.Get(kIdx)
		v := t.stringTable.Get(vIdx)
		newK, newV, shouldReplace := f(k, v)
		if shouldReplace {
			modified = true
			// TODO: Don't bother deleting from string table for now
			// It might be better, but also might be slower
			delete(t.meta, kIdx)
			newKIdx := t.stringTable.Update(kIdx, newK)
			newVIdx := t.stringTable.Update(vIdx, newV)
			t.meta[newKIdx] = newVIdx
		}
	}
	return modified
}

func (t *TableSpan) SetMeta(k string, v string) {
	// TODO: we can delete old entries here
	kIdx, ok := t.stringTable.GetIndex(k)
	if !ok {
		kIdx = t.stringTable.Add(k)
	}
	vIdx := t.stringTable.Add(v)
	t.meta[kIdx] = vIdx
}

func (t *TableSpan) DeleteMeta(k string) {
	// TODO: we can delete old string table entries here
	kIdx, ok := t.stringTable.GetIndex(k)
	if !ok {
		return
	}
	delete(t.meta, kIdx)
}

func (t *TableSpan) Metrics(k string) (float64, bool) {
	kIdx, ok := t.stringTable.GetIndex(k)
	if !ok {
		return 0, false
	}
	v, ok := t.metrics[kIdx]
	return v, ok
}

func (t *TableSpan) SetMetrics(k string, v float64) {
	// TODO: we can delete old entries here
	kIdx, ok := t.stringTable.GetIndex(k)
	if !ok {
		kIdx = t.stringTable.Add(k)
	}
	t.metrics[kIdx] = v
}

func (t *TableSpan) ForMetrics(f func(string, float64) (string, float64, bool)) bool {
	modified := false
	for kIdx, v := range t.metrics {
		k := t.stringTable.Get(kIdx)
		newK, newV, shouldReplace := f(k, v)
		if shouldReplace {
			modified = true
			// TODO: Don't bother deleting from string table for now
			// It might be better, but also might be slower
			delete(t.meta, kIdx)
			newKIdx := t.stringTable.Update(kIdx, newK)
			t.metrics[newKIdx] = newV
		}
	}
	return modified
}

func (t *TableSpan) DeleteMetric(k string) {
	// TODO: we can delete old string table entries here
	kIdx, ok := t.stringTable.GetIndex(k)
	if !ok {
		return
	}
	delete(t.metrics, kIdx)
}
