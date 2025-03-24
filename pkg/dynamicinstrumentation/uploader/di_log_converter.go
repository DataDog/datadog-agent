// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package uploader

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"

	"github.com/google/uuid"
)

// NewDILog creates a new snapshot upload based on the event and relevant process
func NewDILog(procInfo *ditypes.ProcessInfo, event *ditypes.DIEvent) *ditypes.SnapshotUpload {
	if procInfo == nil {
		log.Infof("Process with pid %d not found, ignoring event", event.PID)
		return nil
	}
	probe := procInfo.GetProbe(event.ProbeID)
	if probe == nil {
		log.Info("Probe ID not found, ignoring event", event.ProbeID)
		return nil
	}

	snapshotID, _ := uuid.NewUUID()
	argDefs := getFunctionArguments(procInfo, probe)
	var captures ditypes.Captures
	if probe.InstrumentationInfo.InstrumentationOptions.CaptureParameters {
		captures = convertCaptures(argDefs, event.Argdata)
	} else {
		captures = reportCaptureError(argDefs)
	}

	capturesJSON, _ := json.Marshal(captures)
	stackTrace, err := parseStackTrace(procInfo, event.StackPCs)
	if err != nil {
		log.Infof("event from pid/probe %d/%s does not include stack trace: %s\n", event.PID, event.ProbeID, err)
	}
	return &ditypes.SnapshotUpload{
		Service:  probe.ServiceName,
		Message:  fmt.Sprintf("%s %s", probe.FuncName, capturesJSON),
		DDSource: "dd_debugger",
		DDTags:   "",
		Debugger: struct {
			ditypes.Snapshot `json:"snapshot"`
		}{
			Snapshot: ditypes.Snapshot{
				ID:              &snapshotID,
				Timestamp:       time.Now().UnixNano() / int64(time.Millisecond),
				Language:        "go",
				ProbeInSnapshot: convertProbe(probe),
				Captures:        captures,
				Stack:           stackTrace,
			},
		},
		Duration: 0,
	}
}

func convertProbe(probe *ditypes.Probe) ditypes.ProbeInSnapshot {
	module, function := parseFuncName(probe.FuncName)
	return ditypes.ProbeInSnapshot{
		ID: getProbeUUID(probe.ID),
		ProbeLocation: ditypes.ProbeLocation{
			Method: function,
			Type:   module,
		},
	}
}

func convertCaptures(defs []*ditypes.Parameter, captures []*ditypes.Param) ditypes.Captures {
	return ditypes.Captures{
		Entry: &ditypes.Capture{
			Arguments: convertArgs(defs, captures),
		},
	}
}

func reportCaptureError(defs []*ditypes.Parameter) ditypes.Captures {
	notCapturedReason := "type unsupported"

	args := make(map[string]*ditypes.CapturedValue)
	for _, def := range defs {
		args[def.Name] = &ditypes.CapturedValue{
			Type:              def.Type,
			NotCapturedReason: notCapturedReason,
		}
	}
	return ditypes.Captures{
		Entry: &ditypes.Capture{
			Arguments: args,
		},
	}
}

func convertArgs(defs []*ditypes.Parameter, captures []*ditypes.Param) map[string]*ditypes.CapturedValue {
	args := make(map[string]*ditypes.CapturedValue)

	for idx, def := range defs {
		argName := def.Name
		if argName == "" {
			argName = fmt.Sprintf("arg_%d", idx)
		}

		var capture *ditypes.Param
		if idx < len(captures) {
			capture = captures[idx]
		}

		if capture == nil {
			// No capture for this def, check for not capture reason
			args[argName] = &ditypes.CapturedValue{
				Type: def.Type,
			}
			if def.DoNotCapture && def.NotCaptureReason != 0 {
				args[argName].NotCapturedReason = def.NotCaptureReason.String()
			}
			continue
		}

		cv := &ditypes.CapturedValue{
			Type: def.Type,
		}

		if capture.ValueStr != "" || capture.Type == "string" {
			valueCopy := capture.ValueStr
			cv.Value = &valueCopy
		}

		// Handle nested fields if both def and capture have them
		if capture.Fields != nil && def.ParameterPieces != nil {
			// For slice types, use convertSlice helper which already exists
			if uint(capture.Kind) == uint(reflect.Slice) {
				args[argName] = convertSlice(capture)
			} else {
				// For struct types, recursively process fields
				cv.Fields = convertArgs(def.ParameterPieces, capture.Fields)
				args[argName] = cv
			}
		} else {
			// No nested fields or already handled above
			args[argName] = cv
		}
	}

	// Handle extra captures not in defs
	for idx, capture := range captures {
		if idx >= len(defs) && capture != nil {
			argName := fmt.Sprintf("arg_%d", idx)
			cv := &ditypes.CapturedValue{
				Type: capture.Type,
			}
			if capture.ValueStr != "" || capture.Type == "string" {
				valueCopy := capture.ValueStr
				cv.Value = &valueCopy
			}
			// Don't recursively process fields for captures not in defs
			args[argName] = cv
		}
	}
	return args
}

func convertSlice(capture *ditypes.Param) *ditypes.CapturedValue {
	defs := []*ditypes.Parameter{}
	for i := range capture.Fields {
		var (
			fieldType string
			fieldKind uint
			fieldSize int64
		)
		if capture.Fields[i] != nil {
			fieldType = capture.Fields[i].Type
			fieldKind = uint(capture.Fields[i].Kind)
			fieldSize = int64(capture.Fields[i].Size)
		}
		defs = append(defs, &ditypes.Parameter{
			Name:      fmt.Sprintf("[%d]%s", i, fieldType),
			Type:      fieldType,
			Kind:      fieldKind,
			TotalSize: fieldSize,
		})
	}
	sliceValue := &ditypes.CapturedValue{
		Type:   capture.Type,
		Fields: convertArgs(defs, capture.Fields),
	}
	return sliceValue
}

func parseFuncName(funcName string) (string, string) {
	parts := strings.Split(funcName, ".")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", funcName
}

func getFunctionArguments(proc *ditypes.ProcessInfo, probe *ditypes.Probe) []*ditypes.Parameter {
	return proc.TypeMap.Functions[probe.FuncName]
}

func getProbeUUID(probeID string) string {
	// the RC config ID format is datadog/<org_id>/<product>/<probe_type>_<probe_uuid>/<hash>
	// if we fail to parse it, we just return the original probeID string
	parts := strings.Split(probeID, "/")
	if len(parts) != 5 {
		return probeID
	}
	idPart := parts[len(parts)-2]
	parts = strings.Split(idPart, "_")
	if len(parts) != 2 {
		return probeID
	}
	// we could also validate that the extracted string is a valid UUID,
	// but it's not necessary since we tolerate IDs that don't parse
	return parts[1]
}
