package uploader

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/di/ditypes"

	"github.com/google/uuid"
)

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

func convertCaptures(defs []ditypes.Parameter, captures []*ditypes.Param) ditypes.Captures {
	return ditypes.Captures{
		Entry: &ditypes.Capture{
			Arguments: convertArgs(defs, captures),
		},
	}
}

func reportCaptureError(defs []ditypes.Parameter) ditypes.Captures {
	args := make(map[string]*ditypes.CapturedValue)
	for _, def := range defs {
		args[def.Name] = &ditypes.CapturedValue{
			Type:              def.Type,
			NotCapturedReason: "Failed to instrument, type is unsupported or too complex",
		}
	}
	return ditypes.Captures{
		Entry: &ditypes.Capture{
			Arguments: args,
		},
	}
}

func convertArgs(defs []ditypes.Parameter, captures []*ditypes.Param) map[string]*ditypes.CapturedValue {
	args := make(map[string]*ditypes.CapturedValue)
	for idx, capture := range captures {
		var argName string
		if idx < len(defs) {
			argName = defs[idx].Name
		} else {
			argName = fmt.Sprintf("arg_%d", idx)
		}

		if capture == nil {
			continue
		}

		cv := &ditypes.CapturedValue{Type: capture.Type}
		if capture.ValueStr != "" || capture.Type == "string" {
			// we make a copy of the string so the pointer isn't overwritten in the loop
			valueCopy := capture.ValueStr
			cv.Value = &valueCopy
		}
		if capture.Fields != nil && idx < len(defs) {
			cv.Fields = convertArgs(defs[idx].ParameterPieces, capture.Fields)
		}
		args[argName] = cv
	}
	return args
}

func parseFuncName(funcName string) (string, string) {
	parts := strings.Split(funcName, ".")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", funcName
}

func getFunctionArguments(proc *ditypes.ProcessInfo, probe *ditypes.Probe) []ditypes.Parameter {
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
