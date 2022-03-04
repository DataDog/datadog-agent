package stats

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"unsafe"
)

// Reporter reports stats
type Reporter struct {
	stats map[string]statsInfo
	value reflect.Value
}

type statsInfo struct {
	idx    int
	atomic bool
}

const statsTag = "stats"

// NewReporter create a new Reporter
func NewReporter(v interface{}) (Reporter, error) {
	r := Reporter{
		stats: map[string]statsInfo{},
		value: reflect.ValueOf(v).Elem(),
	}

	tv := r.value.Type()
	for f := 0; f < tv.NumField(); f++ {
		field := tv.Field(f)
		if tagValue, ok := field.Tag.Lookup(statsTag); ok {
			atomic := tagValue == "atomic"
			if !isTypeSupported(field, atomic) {
				return Reporter{}, fmt.Errorf("unsupported type %+v for field %s", field.Type.Kind(), field.Name)
			}

			r.stats[toSnakeCase(field.Name)] = statsInfo{idx: f, atomic: atomic}
		}
	}

	return r, nil
}

func isTypeSupported(f reflect.StructField, atomic bool) bool {
	if atomic {
		switch f.Type.Kind() {
		case reflect.Int32, reflect.Int64, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return true
		}

		return false
	}

	switch f.Type.Kind() {
	case reflect.Int, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int8,
		reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint8, reflect.Uintptr:
		return true
	}

	return false
}

// Report reports stats from the stats object
func (r Reporter) Report() map[string]interface{} {
	stats := make(map[string]interface{}, len(r.stats))
	for name, si := range r.stats {
		var v interface{}
		f := r.value.Field(si.idx)
		if si.atomic {
			stats[name] = loadAtomic(f)
			continue
		}

		switch f.Kind() {
		case reflect.Int:
			v = int(f.Int())
		case reflect.Int8:
			v = int8(f.Int())
		case reflect.Int16:
			v = int16(f.Int())
		case reflect.Int32:
			v = int32(f.Int())
		case reflect.Int64:
			v = int64(f.Int())
		case reflect.Uint:
			v = uint(f.Uint())
		case reflect.Uint8:
			v = uint8(f.Uint())
		case reflect.Uint16:
			v = uint16(f.Uint())
		case reflect.Uint32:
			v = uint32(f.Uint())
		case reflect.Uint64:
			v = uint64(f.Uint())
		}
		stats[name] = v
	}

	return stats
}

func loadAtomic(f reflect.Value) interface{} {
	switch f.Kind() {
	case reflect.Int32:
		return atomic.LoadInt32((*int32)(unsafe.Pointer(f.UnsafeAddr())))
	case reflect.Int64:
		return atomic.LoadInt64((*int64)(unsafe.Pointer(f.UnsafeAddr())))
	case reflect.Uint:
		return atomic.LoadUintptr((*uintptr)(unsafe.Pointer(f.UnsafeAddr())))
	case reflect.Uint32:
		return atomic.LoadUint32((*uint32)(unsafe.Pointer(f.UnsafeAddr())))
	case reflect.Uint64:
		return atomic.LoadUint64((*uint64)(unsafe.Pointer(f.UnsafeAddr())))
	}

	return nil
}

// from https://gist.github.com/stoewer/fbe273b711e6a06315d19552dd4d33e6
var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
