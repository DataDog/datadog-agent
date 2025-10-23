// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cgo

package jmxclient

/*
#cgo LDFLAGS: -L${SRCDIR}/../../../../dev/lib -ljmxclient
#include <stdlib.h>

// TODO(remy): use a header file instead
// graal things
struct __graal_isolate_t;
typedef struct __graal_isolate_t graal_isolate_t;
struct __graal_isolatethread_t;
typedef struct __graal_isolatethread_t graal_isolatethread_t;

typedef unsigned long __graal_uword;

enum { __graal_create_isolate_params_version = 4 };
struct __graal_create_isolate_params_t {
    int version;
    __graal_uword  reserved_address_space_size;
    const char    *auxiliary_image_path;
    __graal_uword  auxiliary_image_reserved_space_size;
    int            _reserved_1;
    char         **_reserved_2;
    int            pkey;
    char           _reserved_3;
    char           _reserved_4;
    char           _reserved_5;
};
typedef struct __graal_create_isolate_params_t graal_create_isolate_params_t;

int graal_create_isolate(graal_create_isolate_params_t* params, graal_isolate_t** isolate, graal_isolatethread_t** thread);
int graal_attach_thread(graal_isolate_t* isolate, graal_isolatethread_t** thread);
int graal_detach_thread(graal_isolatethread_t* thread);

// jmxclient things
int connect_jvm(void*, char*, int);
int prepare_beans(void*, int, char*);
char* collect_beans(void*, int);
int close_jvm(void*, int);
void free_string(void*, char*);
int cleanup_all(void*);
*/
import "C"
import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

var (
	sharedWrapper     *JmxClientWrapper
	sharedWrapperOnce sync.Once
	sharedWrapperErr  error
)

// JmxClientWrapper wraps the CGo calls to the JmxClient library
type JmxClientWrapper struct {
	isolate *C.graal_isolate_t
}

// GetSharedWrapper returns the singleton JmxClientWrapper instance
// The wrapper is initialized once and shared across all check instances
func GetSharedWrapper() (*JmxClientWrapper, error) {
	sharedWrapperOnce.Do(func() {
		sharedWrapper = newJmxClientWrapper()
		if sharedWrapper == nil {
			sharedWrapperErr = fmt.Errorf("failed to create JmxClient wrapper: GraalVM isolate initialization failed")
		}
	})
	return sharedWrapper, sharedWrapperErr
}

// newJmxClientWrapper creates a new wrapper instance (internal use only)
func newJmxClientWrapper() *JmxClientWrapper {
	var isolate *C.graal_isolate_t
	var thread *C.graal_isolatethread_t

	// Create isolate - the thread is needed for initialization but not stored
	errorCode := C.graal_create_isolate(nil, &isolate, &thread)
	fmt.Println("graal create isolate:", errorCode)
	if errorCode < 0 || isolate == nil || thread == nil {
	    return nil
	}

	// Detach the initialization thread - we'll attach new threads as needed
	C.graal_detach_thread(thread)

	return &JmxClientWrapper{
		isolate: isolate,
	}
}

// attachThread attaches the current OS thread to the GraalVM isolate
// Returns the thread pointer and a cleanup function
// The cleanup function MUST be called (typically via defer) to detach the thread
func (w *JmxClientWrapper) attachThread() (*C.graal_isolatethread_t, func(), error) {
	var thread *C.graal_isolatethread_t

	if errorCode := C.graal_attach_thread(w.isolate, &thread); errorCode != 0 {
		return nil, nil, fmt.Errorf("failed to attach thread to isolate, error code: %d", errorCode)
	}

	cleanup := func() {
		C.graal_detach_thread(thread)
	}

	return thread, cleanup, nil
}

// ConnectJVM connects to a JVM instance
// Returns a connection handle (session ID) on success, or an error
func (w *JmxClientWrapper) ConnectJVM(host string, port int) (int, error) {
	thread, cleanup, err := w.attachThread()
	if err != nil {
		return 0, err
	}
	defer cleanup()

	cHost := C.CString(host)
	defer C.free(unsafe.Pointer(cHost))

	sessionID := int(C.connect_jvm(unsafe.Pointer(thread), cHost, C.int(port)))
	if sessionID < 0 {
		return 0, fmt.Errorf("failed to connect to JVM at %s:%d, error code: %d", host, port, sessionID)
	}

	return sessionID, nil
}

// PrepareBeans configures which beans to collect for a given session
// beansConfig should be a JSON string describing the bean collection configuration
func (w *JmxClientWrapper) PrepareBeans(sessionID int, beansConfig string) error {
	thread, cleanup, err := w.attachThread()
	if err != nil {
		return err
	}
	defer cleanup()

	cConfig := C.CString(beansConfig)
	defer C.free(unsafe.Pointer(cConfig))

	result := int(C.prepare_beans(unsafe.Pointer(thread), C.int(sessionID), cConfig))
	if result != 0 {
		return fmt.Errorf("failed to prepare beans for session %d, error code: %d", sessionID, result)
	}

	return nil
}

// CollectBeans collects metrics from configured beans
// Returns a JSON string with the collected metrics
func (w *JmxClientWrapper) CollectBeans(sessionID int) (string, error) {
	thread, cleanup, err := w.attachThread()
	if err != nil {
		return "", err
	}
	defer cleanup()

	cResult := C.collect_beans(unsafe.Pointer(thread), C.int(sessionID))
	if cResult == nil {
		return "", fmt.Errorf("failed to collect beans for session %d", sessionID)
	}

	result := C.GoString(cResult)
	C.free_string(unsafe.Pointer(thread), cResult)

	return result, nil
}

// BeanAttribute represents a single attribute name-value pair
type BeanAttribute struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// BeanData represents a collected JMX bean with its attributes
type BeanData struct {
	Path       string          `json:"path"`
	Success    bool            `json:"success"`
	Attributes []BeanAttribute `json:"attributes"`
	Attribute  string          `json:"attribute"`
	Type       string          `json:"type"`
}

// CollectBeansAsStructs collects metrics and returns them as a slice of BeanData
func (w *JmxClientWrapper) CollectBeansAsStructs(sessionID int) ([]BeanData, error) {
	jsonStr, err := w.CollectBeans(sessionID)
	if err != nil {
		return nil, err
	}

	var result []BeanData
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse collected beans JSON: %w", err)
	}

	return result, nil
}

// CloseJVM closes a specific JVM connection
func (w *JmxClientWrapper) CloseJVM(sessionID int) error {
	thread, cleanup, err := w.attachThread()
	if err != nil {
		return err
	}
	defer cleanup()

	result := int(C.close_jvm(unsafe.Pointer(thread), C.int(sessionID)))
	if result != 0 {
		return fmt.Errorf("failed to close JVM session %d, error code: %d", sessionID, result)
	}

	return nil
}

// CleanupAll cleans up all JVM connections and resources
func (w *JmxClientWrapper) CleanupAll() error {
	thread, cleanup, err := w.attachThread()
	if err != nil {
		return err
	}
	defer cleanup()

	result := int(C.cleanup_all(unsafe.Pointer(thread)))
	if result != 0 {
		return errors.New("failed to cleanup all JVM connections")
	}

	return nil
}
