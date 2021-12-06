// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pb

import (
	"sync"
)

var (
	mu             sync.RWMutex // guard metahooks
	metahook       func(_, v string) string
	metastructhook func(k string, v []byte) []byte
)

// SetMetaHooks registers callbacks which will run upon decoding each map entry
// in the span's Meta and MetaStruct fields. The hooks have the opportunity to
// alter the value that is assigned to span.Meta[k] and span.MetaStruct[k] at
// decode time. By default, if no hook is defined, the behaviour is
// span.Meta[k] = v and span.MetaStruct[k] = v.
func SetMetaHooks(hook func(k, v string) string, structhook func(k string, v []byte) []byte) {
	mu.Lock()
	defer mu.Unlock()
	metahook = hook
	metastructhook = structhook
}

// HasMetaHooks returns true if there is an active hook on Meta or MetaStruct
// fields.
func HasMetaHooks() bool {
	mu.RLock()
	defer mu.RUnlock()
	return metahook != nil || metastructhook != nil
}

// MetaHook returns the active meta hook. A MetaHook is a function which is ran
// for each span.Meta[k] = v value and has the opportunity to alter the final v.
func MetaHook() (hook func(k, v string) string, ok bool) {
	mu.RLock()
	defer mu.RUnlock()
	return metahook, metahook != nil
}

// MetaStructHook returns the active meta struct hook. A MetaStructHook is a
// function which is ran for each span.MetaStruct[k] = v value and has the
// opportunity to alter the final v.
func MetaStructHook() (hook func(k string, v []byte) []byte, ok bool) {
	mu.RLock()
	defer mu.RUnlock()
	return metastructhook, metastructhook != nil
}
