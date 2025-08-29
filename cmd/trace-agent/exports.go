// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build cshared
// +build cshared

package main

//// #include <stdlib.h>
/*
// bool is a typedef for unsigned char used as a boolean type.
// Values: 1 for true, 0 for false.
typedef unsigned char bool;

// byte is a typedef for unsigned char used as a byte type.
typedef unsigned char byte;

// key_value_pair represents a single string-based key-value pair, commonly used for tags and metadata.
// Fields:
//   - key: A C-string containing the key.
//   - value: A C-string containing the value.
typedef struct {
    char* key;
    char* value;
} key_value_pair;

// key_value_pair_array represents an array of key-value pairs.
// Fields:
//   - data: Pointer to an array of key_value_pair.
//   - len: The length of the array.
//
// Used for passing lists of environment variables, tags, and other string metadata.
typedef struct {
    key_value_pair* data;
    size_t len;
} key_value_pair_array;

// byte_array represents a byte array.
// Fields:
//   - data: Pointer to the byte data.
//   - len: The length of the byte array.
//
// Used for passing raw binary data, such as serialized traces.
typedef struct {
	byte* data;
	size_t len;
} byte_array;
*/
import "C"
import (
	"context"
	"fmt"
	"sync/atomic"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/agent/autoexit"
	"github.com/DataDog/datadog-agent/comp/agent/autoexit/autoexitimpl"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/configsync/configsyncimpl"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logtracefx "github.com/DataDog/datadog-agent/comp/core/log/fx-trace"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	optionalRemoteTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-optional-remote"
	taggerTypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/statsd"
	traceagent "github.com/DataDog/datadog-agent/comp/trace/agent/def"
	traceagentimpl "github.com/DataDog/datadog-agent/comp/trace/agent/impl"
	zstdfx "github.com/DataDog/datadog-agent/comp/trace/compression/fx-zstd"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type (
	channels struct {
		initDone    chan api.Receiver
		stopChannel chan struct{}
		doneChannel chan bool
	}
)

var (
	initialized   atomic.Bool
	ch            *channels    = nil
	traceReceiver api.Receiver = nil
)

const (
	key_value_pair_size = C.size_t(unsafe.Sizeof(C.key_value_pair{}))
)

//export initialize
func initialize() C.bool {
	if initialized.Load() {
		fmt.Println("Trace Agent shared library already initialized")
		return toBool(true)
	}

	ch = initializeFx()
	traceReceiver = <-ch.initDone

	fmt.Println("Trace Agent shared library initialized")
	initialized.Store(true)
	return toBool(true)
}

//export stop
func stop() C.bool {
	if !initialized.Load() {
		fmt.Println("Trace Agent shared library not initialized")
		return toBool(false)
	}

	ch.stopChannel <- struct{}{}
	res := <-ch.doneChannel

	initialized.Store(false)
	fmt.Println("Trace Agent shared library stopped")
	return toBool(res)
}

//export submit_traces
func submit_traces(version *C.char, headers C.key_value_pair_array, payload C.byte_array) {
	if !initialized.Load() {
		fmt.Println("Trace Agent shared library not initialized")
		return
	}

	if traceReceiver == nil {
		fmt.Println("Trace Agent receiver is nil")
		return
	}

	if receiver := traceReceiver.(*api.BypassReceiver); receiver != nil {
		g_version := C.GoString(version)
		g_headers := map[string]string{}
		for i := C.size_t(0); i < headers.len; i++ {
			kvp := *(*C.key_value_pair)(unsafe.Add(unsafe.Pointer(headers.data), i*key_value_pair_size))
			key := C.GoString(kvp.key)
			value := C.GoString(kvp.value)
			g_headers[key] = value
		}
		g_payload := C.GoBytes(unsafe.Pointer(payload.data), C.int(payload.len))
		err := receiver.SubmitTraces(context.Background(), api.Version(g_version), g_headers, g_payload)
		if err != nil {
			fmt.Printf("Error submitting traces: %v\n", err)
		} else {
			fmt.Println("Submitted traces successfully")
		}
	}
}

// initializeFx initializes the fx app in a goroutine and returns the channels
func initializeFx() *channels {
	channel := &channels{
		initDone:    make(chan api.Receiver, 1),
		stopChannel: make(chan struct{}),
		doneChannel: make(chan bool, 1),
	}

	ctx := context.WithValue(context.Background(), "bypass-receiver", "true")

	go func() {
		err := fxutil.OneShot(
			func(tc traceagent.Component) {
				channel.initDone <- tc.GetReceiver()
				<-channel.stopChannel
			},
			fx.Provide(func() context.Context { return ctx }), // fx.Supply(ctx) fails with a missing type error.
			fx.Supply(coreconfig.NewAgentParams("", coreconfig.WithFleetPoliciesDirPath(""))),
			secretsfx.Module(),
			fx.Provide(func(comp secrets.Component) option.Option[secrets.Component] {
				return option.New[secrets.Component](comp)
			}),
			fx.Supply(secrets.NewEnabledParams()),
			telemetryimpl.Module(),
			coreconfig.Module(),
			fx.Provide(func() log.Params {
				return log.ForDaemon("TRACE", "apm_config.log_file", config.DefaultLogFilePath)
			}),
			logtracefx.Module(),
			autoexitimpl.Module(),
			statsd.Module(),
			optionalRemoteTaggerfx.Module(
				tagger.OptionalRemoteParams{
					// We disable the remote tagger *only* if we detect that the
					// trace-agent is running in the Azure App Services (AAS)
					// Extension. The Extension only includes a trace-agent and the
					// dogstatsd binary, and cannot include the core agent. We know
					// that we do not need the container tagging provided by the
					// remote tagger in this environment, so we can use the noop
					// tagger instead.
					Disable: serverlessenv.IsAzureAppServicesExtension,
				},
				tagger.RemoteParams{
					RemoteTarget: func(c coreconfig.Component) (string, error) {
						return fmt.Sprintf(":%v", c.GetInt("cmd_port")), nil
					},
					RemoteFilter: taggerTypes.NewMatchAllFilter(),
				}),
			fx.Invoke(func(_ config.Component) {}),
			// Required to avoid cyclic imports.
			fx.Provide(func(cfg config.Component) telemetry.TelemetryCollector { return telemetry.NewCollector(cfg.Object()) }),
			fx.Supply(&traceagentimpl.Params{
				CPUProfile:  "",
				MemProfile:  "",
				PIDFilePath: "",
			}),
			zstdfx.Module(),
			trace.Bundle(),
			ipcfx.ModuleReadWrite(),
			configsyncimpl.Module(configsyncimpl.NewDefaultParams()),
			// Force the instantiation of the components
			fx.Invoke(func(_ traceagent.Component, _ autoexit.Component) {}),
		)

		if err != nil {
			fmt.Printf("Error initializing Trace Agent fx app: %v\n", err)
			channel.initDone <- nil
			channel.doneChannel <- false
			return
		}

		channel.doneChannel <- true
	}()

	return channel
}

// toBool converts a Go bool to a C.bool (0 or 1).
func toBool(value bool) C.bool {
	if value {
		return C.bool(1)
	}
	return C.bool(0)
}
