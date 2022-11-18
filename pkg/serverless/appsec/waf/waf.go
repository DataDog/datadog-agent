// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo && !windows && (amd64 || arm64) && (linux || darwin)

package waf

// #include <stdlib.h>
// #include <string.h>
// #include "ddwaf.h"
// // Forward declaration of the Go function go_ddwaf_object_free which is a Go
// // function defined and exported into C by CGO in this file.
// // This allows to reference this symbol with the C wrapper and pass its
// // pointer to ddwaf_context_init.
// void go_ddwaf_object_free(ddwaf_object*);
// #cgo CFLAGS: -I${SRCDIR}/include
// #cgo linux,amd64 LDFLAGS: -L${SRCDIR}/lib/linux-amd64 -lddwaf -lm -ldl -lrt -Wl,-rpath=/lib64:/usr/lib64:/usr/local/lib64:/lib:/usr/lib:/usr/local/lib
// #cgo linux,arm64 LDFLAGS: -L${SRCDIR}/lib/linux-arm64 -lddwaf -lm -ldl -lrt -Wl,-rpath=/lib64:/usr/lib64:/usr/local/lib64:/lib:/usr/lib:/usr/local/lib
// #cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/lib/darwin-amd64 -lddwaf -lstdc++
// #cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/lib/darwin-arm64 -lddwaf -lstdc++
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"sync"
	"time"
	"unicode"
	"unsafe"

	"go.uber.org/atomic"

	// Do not remove the following imports which allow supporting package
	// vendoring by properly copying all the files needed by CGO: the libddwaf
	// header file and the static libraries.
	_ "github.com/DataDog/datadog-agent/pkg/serverless/appsec/waf/include"
	_ "github.com/DataDog/datadog-agent/pkg/serverless/appsec/waf/lib/darwin-amd64"
	_ "github.com/DataDog/datadog-agent/pkg/serverless/appsec/waf/lib/darwin-arm64"
	_ "github.com/DataDog/datadog-agent/pkg/serverless/appsec/waf/lib/linux-amd64"
	_ "github.com/DataDog/datadog-agent/pkg/serverless/appsec/waf/lib/linux-arm64"
)

var wafVersion = getWAFVersion()

// Health allows knowing if the WAF can be used. It returns a nil error when the WAF library is healthy.
// Otherwise, it returns an error describing the issue.
func Health() error {
	return nil
}

// Version returns the current version of the WAF
func Version() string {
	return wafVersion
}

// Handle represents an instance of the WAF for a given ruleset.
type Handle struct {
	// Instance of the WAF
	handle C.ddwaf_handle

	// Lock-less reference counter avoiding blocking calls to the Close() method
	// while WAF contexts are still using the WAF handle. Instead, we let the
	// release actually happen only when the reference counter reaches 0.
	// This can happen either from a request handler calling its WAF context's
	// Close() method, or either from the appsec instance calling the WAF
	// handle's Close() method when creating a new WAF handle with new rules.
	// Note that this means several instances of the WAF can exist at the same
	// time with their own set of rules. This choice was done to be able to
	// efficiently update the security rules concurrently, without having to
	// block the request handlers for the time of the security rules update.
	refCounter *atomicRefCounter

	// RWMutex protecting the R/W accesses to the internal rules data (stored
	// in the handle).
	mu sync.RWMutex

	// encoder of Go values into ddwaf objects.
	encoder encoder
	// addresses the WAF rule is expecting.
	addresses []string
	// rulesetInfo holds information about rules initialization
	rulesetInfo RulesetInfo
}

// NewHandle creates a new instance of the WAF with the given JSON rule and key/value regexps for obfuscation.
func NewHandle(jsonRule []byte, keyRegex, valueRegex string) (*Handle, error) {
	var rule interface{}
	if err := json.Unmarshal(jsonRule, &rule); err != nil {
		return nil, fmt.Errorf("could not parse the WAF rule: %v", err)
	}

	// Create a temporary unlimited encoder for the rules
	const intSize = 32 << (^uint(0) >> 63) // copied from recent versions of math.MaxInt
	const maxInt = 1<<(intSize-1) - 1      // copied from recent versions of math.MaxInt
	ruleEncoder := encoder{
		maxDepth:        maxInt,
		maxStringLength: maxInt,
		maxArrayLength:  maxInt,
		maxMapLength:    maxInt,
	}
	wafRule, err := ruleEncoder.encode(rule)
	if err != nil {
		return nil, fmt.Errorf("could not encode the JSON WAF rule into a WAF object: %v", err)
	}
	defer freeWO(wafRule)

	// Run-time encoder limiting the size of the encoded values
	encoder := encoder{
		maxDepth:        C.DDWAF_MAX_CONTAINER_DEPTH,
		maxStringLength: C.DDWAF_MAX_STRING_LENGTH,
		maxArrayLength:  C.DDWAF_MAX_CONTAINER_SIZE,
		maxMapLength:    C.DDWAF_MAX_CONTAINER_SIZE,
	}
	var wafRInfo C.ddwaf_ruleset_info
	keyRegexC, _, err := cstring(keyRegex, encoder.maxStringLength)
	if err != nil {
		return nil, fmt.Errorf("could not convert the obfuscator key regexp string to a C string: %v", err)
	}
	defer cFree(unsafe.Pointer(keyRegexC))
	valueRegexC, _, err := cstring(valueRegex, encoder.maxStringLength)
	if err != nil {
		return nil, fmt.Errorf("could not convert the obfuscator value regexp to a C string: %v", err)
	}
	defer cFree(unsafe.Pointer(valueRegexC))
	wafCfg := C.ddwaf_config{
		limits: struct{ max_container_size, max_container_depth, max_string_length C.uint32_t }{
			max_container_size:  C.uint32_t(encoder.maxArrayLength),
			max_container_depth: C.uint32_t(encoder.maxMapLength),
			max_string_length:   C.uint32_t(encoder.maxStringLength),
		},
		obfuscator: struct{ key_regex, value_regex *C.char }{
			key_regex:   keyRegexC,
			value_regex: valueRegexC,
		},
		free_fn: C.ddwaf_object_free_fn(C.go_ddwaf_object_free),
	}
	defer C.ddwaf_ruleset_info_free(&wafRInfo)
	handle := C.ddwaf_init(wafRule.ctype(), &wafCfg, &wafRInfo)
	if handle == nil {
		return nil, errors.New("could not instantiate the waf rule")
	}
	incNbLiveCObjects()

	// Decode the ruleset information returned by the WAF
	errors, err := decodeErrors((*wafObject)(&wafRInfo.errors))
	if err != nil {
		C.ddwaf_destroy(handle)
		decNbLiveCObjects()
		return nil, err
	}
	rInfo := RulesetInfo{
		Failed:  uint16(wafRInfo.failed),
		Loaded:  uint16(wafRInfo.loaded),
		Version: C.GoString(wafRInfo.version),
		Errors:  errors,
	}
	// Get the addresses the rule listens to
	addresses, err := ruleAddresses(handle)
	if err != nil {
		C.ddwaf_destroy(handle)
		decNbLiveCObjects()
		return nil, err
	}
	return &Handle{
		handle:      handle,
		refCounter:  (*atomicRefCounter)(atomic.NewUint32(1)),
		encoder:     encoder,
		addresses:   addresses,
		rulesetInfo: rInfo,
	}, nil
}

// Increment the ref counter and return true if the handle can be used, false
// otherwise.
func (h *Handle) incrementReferences() bool {
	return h.refCounter.increment() != 0
}

// Decrement the ref counter and release the memory when 0 is reached.
func (h *Handle) decrementReferences() {
	if h.refCounter.decrement() == 0 {
		h.release()
	}
}

// Actual memory release of the WAF handle.
func (h *Handle) release() {
	if h.handle == nil {
		return // already released - only happens if Close() is called more than once
	}
	C.ddwaf_destroy(h.handle)
	decNbLiveCObjects()
	h.handle = nil
}

func ruleAddresses(handle C.ddwaf_handle) ([]string, error) {
	var nbAddresses C.uint32_t
	caddresses := C.ddwaf_required_addresses(handle, &nbAddresses)
	if nbAddresses == 0 {
		return nil, ErrEmptyRuleAddresses
	}
	addresses := make([]string, int(nbAddresses))
	for i := 0; i < len(addresses); i++ {
		addresses[i] = C.GoString(cindexCharPtrArray(caddresses, i))
	}
	return addresses, nil
}

// Addresses returns the list of addresses the WAF rule is expecting.
func (h *Handle) Addresses() []string {
	return h.addresses
}

// RulesetInfo returns the rules initialization metrics for the current WAF handle
func (h *Handle) RulesetInfo() RulesetInfo {
	return h.rulesetInfo
}

// UpdateRuleData updates the data that some rules reference to.
// The given rule data must be a raw JSON string of the form
// [ {rule data #1}, ... {rule data #2} ]
func (h *Handle) UpdateRuleData(jsonData []byte) error {
	var data []interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return fmt.Errorf("could not parse the WAF rule data: %v", err)
	}

	encoded, err := h.encoder.encode(data) // FIXME: double-check with Anil that we are good with the current conversion of integers into strings here
	if err != nil {
		return fmt.Errorf("could not encode the JSON WAF rule data into a WAF object: %v", err)
	}
	defer freeWO(encoded)

	return h.updateRuleData(encoded)
}

// updateRuleData is the critical section of UpdateRuleData
func (h *Handle) updateRuleData(data *wafObject) error {
	// Note about this lock: ddwaf_update_rule_data already is thread-safe to
	// use, but we chose to lock at the goroutine-level instead in order to
	// avoid locking OS threads and therefore prevent many other goroutines from
	// executing during that OS lock. If a goroutine locks due to this handle's
	// RWMutex, another goroutine gets executed on its OS thread.
	h.mu.Lock()
	defer h.mu.Unlock()

	rc := C.ddwaf_update_rule_data(h.handle, data.ctype())
	if rc != C.DDWAF_OK {
		return fmt.Errorf("unexpected error number `%d` while updating the WAF rule data", rc)
	}
	return nil
}

// Close the WAF handle. Note that this call doesn't block until the handle gets
// released but instead let WAF contexts still use it until there's no more (eg.
// when swapping the WAF handle with a new one).
func (h *Handle) Close() {
	h.decrementReferences()
}

// Context is a WAF execution context. It allows to run the WAF incrementally
// by calling it multiple times to run its rules every time new addresses
// become available. Each request must have its own Context.
type Context struct {
	// Instance of the WAF
	handle *Handle
	// Cumulated internal WAF run time - in nanoseconds - for this context.
	totalRuntimeNs AtomicU64
	// Cumulated overall run time - in nanoseconds - for this context.
	totalOverallRuntimeNs AtomicU64
	// Cumulated timeout count for this context.
	timeoutCount AtomicU64

	context C.ddwaf_context
	// Mutex protecting the use of context which is not thread-safe.
	mu sync.Mutex
}

// NewContext creates a new WAF context and increases the number of references
// to the WAF handle.
// A nil value is returned when the WAF handle can no longer be used or the
// WAF context couldn't be created.
func NewContext(handle *Handle) *Context {
	if !handle.incrementReferences() {
		return nil // The WAF handle got released
	}
	context := C.ddwaf_context_init(handle.handle)
	if context == nil {
		handle.decrementReferences()
		return nil
	}
	incNbLiveCObjects()
	return &Context{
		handle:  handle,
		context: context,
	}
}

// Run the WAF with the given Go values and timeout.
func (c *Context) Run(values map[string]interface{}, timeout time.Duration) (matches []byte, actions []string, err error) {
	now := time.Now()
	defer func() {
		dt := time.Since(now)
		c.totalOverallRuntimeNs.Add(uint64(dt.Nanoseconds()))
	}()

	if len(values) == 0 {
		return
	}

	wafValue, err := c.handle.encoder.encode(values)
	if err != nil {
		return nil, nil, err
	}

	return c.run(wafValue, timeout)
}

// run is the critical section of Run
func (c *Context) run(data *wafObject, timeout time.Duration) (matches []byte, actions []string, err error) {
	// Exclusively lock this WAF context for the time of this run
	c.mu.Lock()
	defer c.mu.Unlock()

	// RLock the handle to safely get read access to the WAF handle and prevent concurrent changes of it
	// such as a rules-data update.
	c.handle.mu.RLock()
	defer c.handle.mu.RUnlock()

	var result C.ddwaf_result
	defer freeWAFResult(&result)

	rc := C.ddwaf_run(c.context, data.ctype(), &result, C.uint64_t(timeout/time.Microsecond))

	c.totalRuntimeNs.Add(uint64(result.total_runtime))

	matches, actions, err = goReturnValues(rc, &result)
	if err == ErrTimeout {
		c.timeoutCount.Inc()
	}

	return matches, actions, err
}

// Close the WAF context by releasing its C memory and decreasing the number of
// references to the WAF handle.
func (c *Context) Close() {
	// RUnlock the WAF RWMutex to decrease the count of WAF Contexts using it.
	defer c.handle.decrementReferences()
	C.ddwaf_context_destroy(c.context)
	decNbLiveCObjects()
}

// TotalRuntime returns the cumulated WAF runtime across various run calls within the same WAF context.
// Returned time is in nanoseconds.
func (c *Context) TotalRuntime() (overallRuntimeNs, internalRuntimeNs uint64) {
	return c.totalOverallRuntimeNs.Load(), c.totalRuntimeNs.Load()
}

// TotalTimeouts returns the cumulated amount of WAF timeouts across various run calls within the same WAF context.
func (c *Context) TotalTimeouts() uint64 {
	return c.timeoutCount.Load()
}

// Translate libddwaf return values into return values suitable to a Go program.
// Note that it is possible to have matches or actions even if err is not nil in
// case of a timeout during the WAF call.
func goReturnValues(rc C.DDWAF_RET_CODE, result *C.ddwaf_result) (matches []byte, actions []string, err error) {
	if bool(result.timeout) {
		err = ErrTimeout
	}

	switch rc {
	case C.DDWAF_OK:
		return nil, nil, err

	case C.DDWAF_MATCH:
		if result.data != nil {
			matches = C.GoBytes(unsafe.Pointer(result.data), C.int(C.strlen(result.data)))
		}
		if size := result.actions.size; size > 0 {
			cactions := result.actions.array
			actions = make([]string, size)
			for i := 0; i < int(size); i++ {
				actions[i] = C.GoString(cindexCharPtrArray(cactions, i))
			}
		}
		return matches, actions, err

	default:
		return nil, nil, goRunError(rc)
	}
}

func goRunError(rc C.DDWAF_RET_CODE) error {
	switch rc {
	case C.DDWAF_ERR_INTERNAL:
		return ErrInternal
	case C.DDWAF_ERR_INVALID_OBJECT:
		return ErrInvalidObject
	case C.DDWAF_ERR_INVALID_ARGUMENT:
		return ErrInvalidArgument
	default:
		return fmt.Errorf("unknown waf return code %d", int(rc))
	}
}

func getWAFVersion() string {
	cversion := C.ddwaf_get_version() // static mem pointer returned - no need to free it
	return C.GoString(cversion)
}

// Errors the encoder and decoder can return.
var (
	errMaxDepth         = errors.New("max depth reached")
	errUnsupportedValue = errors.New("unsupported Go value")
	errOutOfMemory      = errors.New("out of memory")
	errInvalidMapKey    = errors.New("invalid WAF object map key")
	errNilObjectPtr     = errors.New("nil WAF object pointer")
)

// isIgnoredValueError returns true if the error is only about ignored Go values
// (errUnsupportedValue or errMaxDepth).
func isIgnoredValueError(err error) bool {
	return err == errUnsupportedValue || err == errMaxDepth
}

// encoder is allows to encode a Go value to a WAF object
type encoder struct {
	// Maximum depth a WAF object can have. Every Go value further this depth is
	// ignored and not encoded into a WAF object.
	maxDepth int
	// Maximum string length. A string longer than this length is truncated to
	// this length.
	maxStringLength int
	// Maximum string length. Everything further this length is ignored.
	maxArrayLength int
	// Maximum map length. Everything further this length is ignored. Given the
	// fact Go maps are unordered, it means WAF map objects created from Go maps
	// larger than this length will have random keys.
	maxMapLength int
}

func (e *encoder) encode(v interface{}) (object *wafObject, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("waf panic: %v", v)
		}
		if err != nil && object != nil {
			freeWO(object)
		}
	}()
	wo := &wafObject{}
	err = e.encodeValue(reflect.ValueOf(v), wo, e.maxDepth)
	if err != nil {
		return nil, err
	}
	return wo, nil
}

func (e *encoder) encodeValue(v reflect.Value, wo *wafObject, depth int) error {
	switch kind := v.Kind(); kind {
	default:
		return errUnsupportedValue

	case reflect.Bool:
		var b string
		if v.Bool() {
			b = "true"
		} else {
			b = "false"
		}
		return e.encodeString(b, wo)

	case reflect.Ptr, reflect.Interface:
		// The traversal of pointer and interfaces is not accounted in the depth
		// as it has no impact on the WAF object depth
		return e.encodeValue(v.Elem(), wo, depth)

	case reflect.String:
		return e.encodeString(v.String(), wo)

	case reflect.Struct:
		if depth < 0 {
			return errMaxDepth
		}
		return e.encodeStruct(v, wo, depth-1)

	case reflect.Map:
		if depth < 0 {
			return errMaxDepth
		}
		return e.encodeMap(v, wo, depth-1)

	case reflect.Array, reflect.Slice:
		if depth < 0 {
			return errMaxDepth
		}
		if v.Type() == reflect.TypeOf([]byte(nil)) {
			return e.encodeString(string(v.Bytes()), wo)
		}
		return e.encodeArray(v, wo, depth-1)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return e.encodeInt64(v.Int(), wo)

	case reflect.Float32, reflect.Float64:
		return e.encodeInt64(int64(math.Round(v.Float())), wo)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return e.encodeUint64(v.Uint(), wo)
	}
}

func (e *encoder) encodeStruct(v reflect.Value, wo *wafObject, depth int) error {
	// Consider the number of struct fields as the WAF map capacity as some
	// struct fields might not be supported and ignored.
	typ := v.Type()
	nbFields := typ.NumField()
	capacity := nbFields
	if capacity > e.maxMapLength {
		capacity = e.maxMapLength
	}
	if err := wo.setMapContainer(C.size_t(capacity)); err != nil {
		return err
	}
	// Encode struct fields
	length := 0
	for i := 0; length < capacity && i < nbFields; i++ {
		field := typ.Field(i)
		// Skip private fields
		fieldName := field.Name
		if len(fieldName) < 1 || unicode.IsLower(rune(fieldName[0])) {
			continue
		}

		mapEntry := wo.index(C.uint64_t(length))
		if err := e.encodeMapKey(reflect.ValueOf(fieldName), mapEntry); isIgnoredValueError(err) {
			continue
		}

		if err := e.encodeValue(v.Field(i), mapEntry, depth); err != nil {
			// Free the map entry in order to free the previously allocated map key
			freeWO(mapEntry)
			if isIgnoredValueError(err) {
				continue
			}
			return err
		}
		length++
	}
	// Update the map length to the actual one
	if length != capacity {
		wo.setLength(C.uint64_t(length))
	}
	return nil
}

func (e *encoder) encodeMap(v reflect.Value, wo *wafObject, depth int) error {
	// Consider the Go map value length the WAF map capacity as some map entries
	// might not be supported and ignored.
	// In this case, the actual map length will be lesser than the Go map value
	// length.
	capacity := v.Len()
	if capacity > e.maxMapLength {
		capacity = e.maxMapLength
	}
	if err := wo.setMapContainer(C.size_t(capacity)); err != nil {
		return err
	}
	// Encode map entries
	length := 0
	for iter := v.MapRange(); iter.Next(); {
		if length == capacity {
			break
		}
		mapEntry := wo.index(C.uint64_t(length))
		if err := e.encodeMapKey(iter.Key(), mapEntry); isIgnoredValueError(err) {
			continue
		}
		if err := e.encodeValue(iter.Value(), mapEntry, depth); err != nil {
			// Free the previously allocated map key
			freeWO(mapEntry)
			if isIgnoredValueError(err) {
				continue
			}
			return err
		}
		length++
	}
	// Update the map length to the actual one
	if length != capacity {
		wo.setLength(C.uint64_t(length))
	}
	return nil
}

func (e *encoder) encodeMapKey(v reflect.Value, wo *wafObject) error {
	for {
		switch v.Kind() {
		default:
			return errUnsupportedValue

		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return errUnsupportedValue
			}
			v = v.Elem()

		case reflect.String:
			ckey, length, err := cstring(v.String(), e.maxStringLength)
			if err != nil {
				return err
			}
			wo.setMapKey(ckey, C.uint64_t(length))
			return nil
		}
	}
}

func (e *encoder) encodeArray(v reflect.Value, wo *wafObject, depth int) error {
	// Consider the array length as a capacity as some array values might not be supported and ignored. In this case,
	// the actual length will be lesser than the Go value length.
	length := v.Len()
	capacity := length
	if capacity > e.maxArrayLength {
		capacity = e.maxArrayLength
	}
	if err := wo.setArrayContainer(C.size_t(capacity)); err != nil {
		return err
	}
	// Walk the array until we successfully added up to "cap" elements or the Go array length was reached
	currIndex := 0
	for i := 0; currIndex < capacity && i < length; i++ {
		if err := e.encodeValue(v.Index(i), wo.index(C.uint64_t(currIndex)), depth); err != nil {
			if isIgnoredValueError(err) {
				continue
			}
			return err
		}
		// The value has been successfully encoded and added to the array
		currIndex++
	}
	// Update the array length to its actual value in case some array values where ignored
	if currIndex != capacity {
		wo.setLength(C.uint64_t(currIndex))
	}
	return nil
}

func (e *encoder) encodeString(str string, wo *wafObject) error {
	cstr, length, err := cstring(str, e.maxStringLength)
	if err != nil {
		return err
	}
	wo.setString(cstr, C.uint64_t(length))
	return nil
}

func (e *encoder) encodeInt64(n int64, wo *wafObject) error {
	// As of libddwaf v1.0.16, it currently expects numbers as strings
	// TODO(Julio-Guerra): clarify with libddwaf when should it be an actual
	//   int64
	return e.encodeString(strconv.FormatInt(n, 10), wo)
}

func (e *encoder) encodeUint64(n uint64, wo *wafObject) error {
	// As of libddwaf v1.0.16, it currently expects numbers as strings
	// TODO(Julio-Guerra): clarify with libddwaf when should it be an actual
	//   uint64
	return e.encodeString(strconv.FormatUint(n, 10), wo)
}

func decodeErrors(wo *wafObject) (map[string]interface{}, error) {
	v, err := decodeMap(wo)
	if err != nil {
		return nil, err
	}
	if len(v) == 0 {
		v = nil // enforce a nil map when the ddwaf map was empty
	}
	return v, nil
}

func decodeObject(wo *wafObject) (v interface{}, err error) {
	if wo == nil {
		return nil, errNilObjectPtr
	}
	switch wo._type {
	case wafUintType:
		return uint64(*wo.uint64ValuePtr()), nil
	case wafIntType:
		return int64(*wo.int64ValuePtr()), nil
	case wafStringType:
		return gostring(*wo.stringValuePtr(), wo.length())
	case wafArrayType:
		return decodeArray(wo)
	case wafMapType: // could be a map or a struct, no way to differentiate
		return decodeMap(wo)
	default:
		return nil, errUnsupportedValue
	}
}

func decodeArray(wo *wafObject) ([]interface{}, error) {
	if wo == nil {
		return nil, errNilObjectPtr
	}
	var err error
	len := wo.length()
	arr := make([]interface{}, len)
	for i := C.uint64_t(0); i < len && err == nil; i++ {
		arr[i], err = decodeObject(wo.index(i))
	}
	return arr, err
}

func decodeMap(wo *wafObject) (map[string]interface{}, error) {
	if wo == nil {
		return nil, errNilObjectPtr
	}
	length := wo.length()
	decodedMap := make(map[string]interface{}, length)
	for i := C.uint64_t(0); i < length; i++ {
		obj := wo.index(i)
		key, err := decodeMapKey(obj)
		if err != nil {
			return nil, err
		}
		val, err := decodeObject(obj)
		if err != nil {
			return nil, err
		}
		decodedMap[key] = val
	}
	return decodedMap, nil
}

func decodeMapKey(wo *wafObject) (string, error) {
	if wo == nil {
		return "", errNilObjectPtr
	}
	if wo.parameterNameLength == 0 || wo.mapKey() == nil {
		return "", errInvalidMapKey
	}
	return gostring(wo.mapKey(), wo.parameterNameLength)
}

const (
	wafUintType    = C.DDWAF_OBJ_UNSIGNED
	wafIntType     = C.DDWAF_OBJ_SIGNED
	wafStringType  = C.DDWAF_OBJ_STRING
	wafArrayType   = C.DDWAF_OBJ_ARRAY
	wafMapType     = C.DDWAF_OBJ_MAP
	wafInvalidType = C.DDWAF_OBJ_INVALID
)

// wafObject is a Go wrapper allowing to create, access and destroy a WAF object
// C structure.
type wafObject C.ddwaf_object

func (v *wafObject) ctype() *C.ddwaf_object { return (*C.ddwaf_object)(v) }

// Return the pointer to the union field. It can be cast to the union type that needs to be accessed.
func (v *wafObject) valuePtr() unsafe.Pointer        { return unsafe.Pointer(&v.anon0[0]) }
func (v *wafObject) arrayValuePtr() **C.ddwaf_object { return (**C.ddwaf_object)(v.valuePtr()) }
func (v *wafObject) int64ValuePtr() *C.int64_t       { return (*C.int64_t)(v.valuePtr()) }
func (v *wafObject) uint64ValuePtr() *C.uint64_t     { return (*C.uint64_t)(v.valuePtr()) }
func (v *wafObject) stringValuePtr() **C.char        { return (**C.char)(v.valuePtr()) }

func (v *wafObject) setUint64(n C.uint64_t) {
	v._type = wafUintType
	*v.uint64ValuePtr() = n
}

func (v *wafObject) setInt64(n C.int64_t) {
	v._type = wafIntType
	*v.int64ValuePtr() = n
}

func (v *wafObject) setString(str *C.char, length C.uint64_t) {
	v._type = wafStringType
	v.nbEntries = C.uint64_t(length)
	*v.stringValuePtr() = str
}

func (v *wafObject) string() *C.char {
	return *v.stringValuePtr()
}

func (v *wafObject) setInvalid() {
	*v = wafObject{}
}

func (v *wafObject) setContainer(typ C.DDWAF_OBJ_TYPE, length C.size_t) error {
	// Allocate the zero'd array.
	var a *C.ddwaf_object
	if length > 0 {
		a = (*C.ddwaf_object)(C.calloc(length, C.sizeof_ddwaf_object))
		if a == nil {
			return ErrOutOfMemory
		}
		incNbLiveCObjects()
		*v.arrayValuePtr() = a
		v.setLength(C.uint64_t(length))
	}
	v._type = typ
	return nil
}

func (v *wafObject) setArrayContainer(length C.size_t) error {
	return v.setContainer(wafArrayType, length)
}

func (v *wafObject) setMapContainer(length C.size_t) error {
	return v.setContainer(wafMapType, length)
}

func (v *wafObject) setMapKey(key *C.char, length C.uint64_t) {
	v.parameterName = key
	v.parameterNameLength = length
}

func (v *wafObject) mapKey() *C.char {
	return v.parameterName
}

func (v *wafObject) setLength(length C.uint64_t) {
	v.nbEntries = length
}

func (v *wafObject) length() C.uint64_t {
	return v.nbEntries
}

func (v *wafObject) index(i C.uint64_t) *wafObject {
	if C.uint64_t(i) >= v.nbEntries {
		panic(errors.New("out of bounds access to waf array"))
	}
	// Go pointer arithmetic equivalent to the C expression `a->value.array[i]`
	base := uintptr(unsafe.Pointer(*v.arrayValuePtr()))
	return (*wafObject)(unsafe.Pointer(base + C.sizeof_ddwaf_object*uintptr(i))) //nolint:govet
}

// Helper functions for testing, where direct cgo import is not allowed
func toCInt64(v int) C.int64_t {
	return C.int64_t(v)
}
func toCUint64(v uint) C.uint64_t {
	return C.uint64_t(v)
}

// nbLiveCObjects is a simple monitoring of the number of C allocations.
// Tests can read the value to check the count is back to 0.
var nbLiveCObjects atomic.Uint64

func incNbLiveCObjects() {
	nbLiveCObjects.Inc()
}

func decNbLiveCObjects() {
	nbLiveCObjects.Dec()
}

// gostring returns the Go version of the C string `str`, copying at most `len` bytes from the original string.
func gostring(str *C.char, len C.uint64_t) (string, error) {
	if str == nil {
		return "", ErrInvalidArgument
	}
	goLen := C.int(len)
	if C.uint64_t(goLen) != len {
		return "", ErrInvalidArgument
	}
	return C.GoStringN(str, goLen), nil
}

// cstring returns the C string of the given Go string `str` with up to maxWAFStringSize bytes, along with the string
// size that was allocated and copied.
func cstring(str string, maxLength int) (*C.char, int, error) {
	// Limit the maximum string size to copy
	l := len(str)
	if l > maxLength {
		l = maxLength
	}
	// Copy the string up to l.
	// The copy is required as the pointer will be stored into the C structures,
	// so using a Go pointer is impossible.
	cstr := C.CString(str[:l])
	if cstr == nil {
		return nil, 0, errOutOfMemory
	}
	incNbLiveCObjects()
	return cstr, l, nil
}

func freeWO(v *wafObject) {
	if v == nil {
		return
	}
	// Free the map key if any
	if key := v.mapKey(); key != nil {
		cFree(unsafe.Pointer(v.parameterName))
	}
	// Free allocated values
	switch v._type {
	case wafInvalidType:
		return
	case wafStringType:
		cFree(unsafe.Pointer(v.string()))
	case wafMapType, wafArrayType:
		freeWOContainer(v)
	}
	// Make the value invalid to make it unusable
	v.setInvalid()
}

func freeWOContainer(v *wafObject) {
	length := v.length()
	for i := C.uint64_t(0); i < length; i++ {
		freeWO(v.index(i))
	}
	if a := *v.arrayValuePtr(); a != nil {
		cFree(unsafe.Pointer(a))
	}
}

func cFree(ptr unsafe.Pointer) {
	C.free(ptr)
	decNbLiveCObjects()
}

// Exported Go function to free ddwaf objects by using freeWO in order to keep
// its dummy but efficient memory kallocation monitoring.
//
//export go_ddwaf_object_free
func go_ddwaf_object_free(v *C.ddwaf_object) {
	freeWO((*wafObject)(v))
}

// Go reimplementation of ddwaf_result_free to avoid yet another CGO call in the
// request hot-path and avoiding it when there are no results to free.
func freeWAFResult(result *C.ddwaf_result) {
	if data := result.data; data != nil {
		C.free(unsafe.Pointer(data))
	}

	if array := result.actions.array; array != nil {
		for i := 0; i < int(result.actions.size); i++ {
			C.free(unsafe.Pointer(cindexCharPtrArray(array, i)))
		}
		C.free(unsafe.Pointer(array))
	}
}

// Helper function to access to i-th element of the given **C.char array.
func cindexCharPtrArray(array **C.char, i int) *C.char {
	// Go pointer arithmetic equivalent to the C expression `array[i]`
	base := uintptr(unsafe.Pointer(array))
	return *(**C.char)(unsafe.Pointer(base + unsafe.Sizeof((*C.char)(nil))*uintptr(i))) //nolint:govet
}

// Atomic reference counter helper initialized at 1 so that 0 is the special
// value meaning that the object was released and is no longer usable.
type atomicRefCounter atomic.Uint32

func (c *atomicRefCounter) unwrap() *atomic.Uint32 {
	return (*atomic.Uint32)(c)
}

// Add delta to reference counter.
// It relies on a CAS spin-loop implementation in order to avoid changing the
// counter when 0 has been reached.
func (c *atomicRefCounter) add(delta uint32) uint32 {
	addr := c.unwrap()
	for {
		current := addr.Load()
		if current == 0 {
			// The object was released
			return 0
		}
		new := current + delta
		if swapped := addr.CompareAndSwap(current, new); swapped {
			return new
		}
	}
}

// Increment the reference counter.
// CAS spin-loop implementation in order to enforce +1 cannot happen when 0 has
// been reached.
func (c *atomicRefCounter) increment() uint32 {
	return c.add(1)
}

// Decrement the reference counter.
// CAS spin-loop implementation in order to enforce +1 cannot happen when 0 has
// been reached.
func (c *atomicRefCounter) decrement() uint32 {
	const d = ^uint32(0)
	return c.add(d)
}
