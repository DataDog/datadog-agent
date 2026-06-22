// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package crashexporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type crashExporter struct {
	cfg    *Config
	client *http.Client
}

func newExporter(cfg *Config) *crashExporter {
	return &crashExporter{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

func (e *crashExporter) ConsumeProfiles(ctx context.Context, profiles pprofile.Profiles) error {
	for i := range profiles.ResourceProfiles().Len() {
		rp := profiles.ResourceProfiles().At(i)
		// Crash profiles are identified by the presence of crash.frames in
		// resource attributes — set by crashreceiver.buildCrashProfile.
		if _, ok := rp.Resource().Attributes().Get("crash.frames"); !ok {
			log.Warn("crash exporter: received a profiles batch without crash.frames, skipping")
			continue
		}
		payload := e.buildPayload(rp)
		attrs := rp.Resource().Attributes()
		pid, _ := attrs.Get("crash.pid")
		comm, _ := attrs.Get("crash.comm")
		signal, _ := attrs.Get("crash.signal")
		log.Infof("crash exporter: sending crash event — pid=%d comm=%s signal=%d", pid.Int(), comm.Str(), signal.Int())
		if err := e.send(ctx, payload); err != nil {
			log.Errorf("crash exporter: failed to send crash event for pid=%d: %v", pid.Int(), err)
			return err
		}
		log.Infof("crash exporter: crash event submitted successfully for pid=%d", pid.Int())
	}
	return nil
}

func (e *crashExporter) buildPayload(rp pprofile.ResourceProfiles) map[string]any {
	attrs := rp.Resource().Attributes()

	get := func(key string) pcommon.Value {
		v, _ := attrs.Get(key)
		return v
	}
	str := func(key string) string { return get(key).Str() }
	num := func(key string) int64 { return get(key).Int() }

	signo := int(num("crash.signal"))
	sigName := sigNames[signo]
	if sigName == "" {
		sigName = fmt.Sprintf("SIG%d", signo)
	}

	frames := decodeFrames(attrs, "crash.frames")

	// Decode non-primary threads from crash.threads.
	var threads []map[string]any
	if threadsVal, ok := attrs.Get("crash.threads"); ok && threadsVal.Type() == pcommon.ValueTypeSlice {
		sl := threadsVal.Slice()
		for i := range sl.Len() {
			entry := sl.At(i)
			if entry.Type() != pcommon.ValueTypeMap {
				continue
			}
			tm := entry.Map()
			name := ""
			if v, ok := tm.Get("name"); ok {
				name = v.Str()
			}
			threadFrames := decodeFrameSlice(tm, "frames")
			threads = append(threads, map[string]any{
				"crashed": false,
				"name":    name,
				"state":   "S",
				"stack": map[string]any{
					"format":     "Datadog Crashtracker 1.0",
					"frames":     threadFrames,
					"incomplete": false,
				},
			})
		}
	}

	// All resource attributes become tags so processor-enriched data
	// (k8s labels, infra tags, etc.) lands on the crash event automatically.
	// crash.* keys are internal and excluded.
	// service and fingerprint are computed by crashreceiver using the same
	// logic as oomtrace/intake, where libpf types are available.
	service := str("crash.service")
	fp := str("crash.fingerprint")
	lang := str("crash.language")

	tags := []string{
		"service:" + service,
		"language_name:" + lang,
		"data_schema_version:1.8",
		"incomplete:false",
		"is_crash:true",
		"uuid:" + uuid.New().String(),
		"from_ebpf:yes",
	}
	if fp != "" {
		tags = append(tags, "fingerprint:"+fp)
	}
	// Append remaining processor-enriched resource attrs (skip crash.* and
	// service.name which we already handled above).
	attrs.Range(func(k string, v pcommon.Value) bool {
		if strings.HasPrefix(k, "crash.") {
			return true
		}
		if v.Type() == pcommon.ValueTypeStr && v.Str() != "" {
			tags = append(tags, k+":"+v.Str())
		}
		return true
	})

	tsMs := num("crash.timestamp_ms")
	if tsMs == 0 {
		tsMs = time.Now().UnixMilli()
	}

	// Build sig_info matching RFC 0013: SIGKILL from OOM is always SI_KERNEL.
	siCode := 0
	siCodeName := ""
	if signo == 9 {
		siCode = 128
		siCodeName = "SI_KERNEL"
	}

	errObj := map[string]any{
		"type":        sigName,
		"message":     crashMessage(signo),
		"is_crash":    true,
		"fingerprint": fp,
		"source_type": "Crashtracking",
		"stack": map[string]any{
			"format": "Datadog Crashtracker 1.0",
			"frames": frames,
		},
	}
	if len(threads) > 0 {
		errObj["threads"] = threads
	}

	return map[string]any{
		"timestamp": tsMs,
		"ddsource":  "crashtracker",
		"ddtags":    strings.Join(tags, ","),
		"error":     errObj,
		"os_info":   collectOSInfo(),
		"sig_info": map[string]any{
			"si_code":                 siCode,
			"si_code_human_readable":  siCodeName,
			"si_signo":                signo,
			"si_signo_human_readable": sigName,
		},
	}
}

func (e *crashExporter) send(ctx context.Context, payload map[string]any) error {
	endpoint := e.cfg.Endpoint
	if endpoint == "stdout" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}
	if strings.HasPrefix(endpoint, "file://") {
		path := strings.TrimPrefix(endpoint, "file://")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		return json.NewEncoder(f).Encode(payload)
	}

	if endpoint == "" {
		site := e.cfg.Site
		endpoint = "https://error-tracking-intake." + site + "/api/v2/errorsintake"
	}

	apiKey := e.cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("DD_API_KEY")
	}
	if apiKey == "" {
		return fmt.Errorf("crash exporter: DD_API_KEY not set")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("crash exporter: unexpected status %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

var sigNames = map[int]string{
	4: "SIGILL", 6: "SIGABRT", 7: "SIGBUS",
	8: "SIGFPE", 9: "SIGKILL", 11: "SIGSEGV",
}

func crashMessage(signo int) string {
	switch signo {
	case 9:
		return "Process killed by the OOM killer"
	default:
		if name, ok := sigNames[signo]; ok {
			return "Process terminated with " + name
		}
		return fmt.Sprintf("Process terminated with signal %d", signo)
	}
}

func (e *crashExporter) Capabilities() any { return nil }

// decodeFrames reads a Slice of Maps from resource attrs and converts to
// []map[string]any, skipping empty strings (mirrors StackFrame omitempty).
func decodeFrames(attrs pcommon.Map, key string) []map[string]any {
	val, ok := attrs.Get(key)
	if !ok || val.Type() != pcommon.ValueTypeSlice {
		return nil
	}
	return decodeSlice(val.Slice())
}

func decodeFrameSlice(m pcommon.Map, key string) []map[string]any {
	val, ok := m.Get(key)
	if !ok || val.Type() != pcommon.ValueTypeSlice {
		return nil
	}
	return decodeSlice(val.Slice())
}

func decodeSlice(sl pcommon.Slice) []map[string]any {
	frames := make([]map[string]any, 0, sl.Len())
	for i := range sl.Len() {
		entry := sl.At(i)
		if entry.Type() != pcommon.ValueTypeMap {
			continue
		}
		frame := map[string]any{}
		entry.Map().Range(func(k string, v pcommon.Value) bool {
			switch v.Type() {
			case pcommon.ValueTypeStr:
				if s := v.Str(); s != "" {
					frame[k] = s
				}
			case pcommon.ValueTypeInt:
				frame[k] = v.Int()
			}
			return true
		})
		frames = append(frames, frame)
	}
	return frames
}

// collectOSInfo gathers host OS information for the os_info payload field.
func collectOSInfo() map[string]any {
	arch := runtime.GOARCH
	bitness := "64-bit"
	if arch == "386" || arch == "arm" {
		bitness = "32-bit"
	}

	var uname unix.Utsname
	version := "unknown"
	if err := unix.Uname(&uname); err == nil {
		b := make([]byte, 0, len(uname.Release))
		for _, c := range uname.Release {
			if c == 0 {
				break
			}
			b = append(b, byte(c))
		}
		version = string(b)
	}

	return map[string]any{
		"architecture": arch,
		"bitness":      bitness,
		"os_type":      "Linux",
		"version":      version,
	}
}

