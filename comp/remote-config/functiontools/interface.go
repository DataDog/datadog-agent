package functiontools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
)

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type Parameters struct {
	Type                 string              `json:"type"`
	Properties           map[string]Property `json:"properties"`
	Required             []string            `json:"required"`
	AdditionalProperties bool                `json:"additionalProperties"`
}

type Description struct {
	Type        string     `json:"type"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Strict      bool       `json:"strict"`
	Parameters  Parameters `json:"parameters"`
}

type FunctionTool struct {
	description string
	properties  map[string]Property
	handler     func(parameters map[string]string) (any, error)
	isExported  bool
}

var registry = map[string]FunctionTool{
	"list_files": {
		description: "Recursively list all files under a directory that end with a given extension (e.g. 'log' or 'txt'). Returns full file paths. Commonly used to inspect logs located under '/var/log'.",
		properties: map[string]Property{
			"directory": {Type: "string", Description: "The directory to search from (search is recursive). For system logs, this is typically '/var/log'."},
			"extension": {Type: "string", Description: "The file extension to match, exclusing the dot, e.g. 'log' or 'txt'."},
		},
		handler: func(parameters map[string]string) (any, error) {
			return listFiles(parameters["directory"], parameters["extension"])
		},
		isExported: true,
	},
	"tail_file": {
		description: "Read the last N lines of a text file (such as a log file).",
		properties: map[string]Property{
			"file_path": {Type: "string", Description: "The full path of the file to tail."},
			"n_lines":   {Type: "integer", Description: "The number of lines to read from the end of the file. Usually 10 lines is enough to get the last events."},
		},
		handler: func(parameters map[string]string) (any, error) {
			return tailFile(parameters["file_path"], parameters["n_lines"])
		},
		isExported: true,
	},
	"get_file": {
		description: "Read the full contents of a file and post them to the callback URL.",
		properties: map[string]Property{
			"file_path": {Type: "string", Description: "The full path of the file to read."},
			"regex":     {Type: "string", Description: "Optional regular expression used to filter the file contents. Use an empty string to return the full file."},
		},
		handler: func(parameters map[string]string) (any, error) {
			return getFile(parameters["file_path"], parameters["regex"])
		},
		isExported: true,
	},
}

type FunctionToolCall struct {
	CallID           string            `json:"call_id"`
	FunctionToolName string            `json:"function_tool_name"`
	Parameters       map[string]string `json:"parameters"`
	Output           any               `json:"output"`
	Error            error             `json:"error,omitempty"`
	CallbackURL      string            `json:"-"` // Internal field, not serialized
	CallbackPath     string            `json:"-"` // Internal field, not serialized
}

func NewCall(task types.AgentTaskConfig) FunctionToolCall {
	callID, _ := task.Config.GetStringArg("call_id")
	functionToolName, _ := task.Config.GetStringArg("function_tool_name")

	rawParams, ok := task.Config.TaskArgs["parameters"]
	// Parameters can be either a map[string]any (from JSON object) or a string (JSON-encoded)
	parameters := make(map[string]string)
	var callbackURL string
	var callbackPath string
	if ok {
		switch p := rawParams.(type) {
		case map[string]any:
			for k, v := range p {
				if str, ok := v.(string); ok {
					// Extract callback_url from parameters if present
					if k == "callback_url" {
						callbackURL = str
						continue // Don't include callback_url in parameters
					}
					if k == "path" {
						callbackPath = str
						continue // Don't include callback path in parameters
					}
					parameters[k] = str
				} else {
					// Convert non-string values to their string representation
					parameters[k] = fmt.Sprintf("%v", v)
				}
			}
		case string:
			// If parameters is a JSON string, unmarshal it
			err := json.Unmarshal([]byte(p), &parameters)
			if err != nil {
				return FunctionToolCall{
					Error: fmt.Errorf("failed to unmarshal parameters: %w", err),
				}
			}
			// Extract callback_url from parameters if present
			if url, ok := parameters["callback_url"]; ok {
				callbackURL = url
				delete(parameters, "callback_url")
			}
			if path, ok := parameters["path"]; ok {
				callbackPath = path
				delete(parameters, "path")
			}
		default:
			return FunctionToolCall{
				Error: fmt.Errorf("unexpected parameters type: %T", rawParams),
			}
		}
	}

	return FunctionToolCall{
		CallID:           callID,
		FunctionToolName: functionToolName,
		Parameters:       applyParameterDefaults(functionToolName, parameters),
		CallbackURL:      callbackURL,
		CallbackPath:     callbackPath,
	}
}

func applyParameterDefaults(functionToolName string, parameters map[string]string) map[string]string {
	if parameters == nil {
		parameters = make(map[string]string)
	}

	switch functionToolName {
	case "get_file":
		if _, ok := parameters["regex"]; !ok {
			parameters["regex"] = ""
		}
	}

	return parameters
}

func (functionToolCall FunctionToolCall) Execute() FunctionToolCall {
	if functionToolCall.Error != nil {
		return FunctionToolCall{
			Error: fmt.Errorf("failed to execute: %w", functionToolCall.Error),
		}
	}
	functionTool, ok := registry[functionToolCall.FunctionToolName]
	if !ok {
		functionToolCall.Error = fmt.Errorf("function tool '%s' not found", functionToolCall.FunctionToolName)
		return functionToolCall
	}
	for name := range registry[functionToolCall.FunctionToolName].properties {
		if _, ok := functionToolCall.Parameters[name]; !ok {
			functionToolCall.Error = fmt.Errorf("parameter '%s' is required", name)
			return functionToolCall
		}
	}
	result, err := functionTool.handler(functionToolCall.Parameters)
	if err != nil {
		functionToolCall.Error = err
		return functionToolCall
	}
	functionToolCall.Output = result
	return functionToolCall
}

func (functionToolCall FunctionToolCall) Send() error {
	if functionToolCall.Error != nil {
		return fmt.Errorf("failed to send result: %w", functionToolCall.Error)
	}
	if functionToolCall.Output == nil {
		return fmt.Errorf("output is empty")
	}
	jsonData, err := json.Marshal(functionToolCall)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := http.Post(
		"http://127.0.0.1:8123/functiontool",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	fmt.Printf("Successfully sent result to webserver (status: %d)\n", resp.StatusCode)
	return nil
}
