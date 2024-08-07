// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package integrations

import (
	"encoding/json"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// New creates a parser that extracts the `ddtags` field from JSON logs, adds
// them as tags to the log, then submits the rest of the log as is. Non-JSON
// logs will be submitted as is.
func New() parsers.Parser {
	return &integrationFileFormat{}
}

type integrationFileFormat struct{}

// Parse implements Parser#Parse
func (p *integrationFileFormat) Parse(msg *message.Message) (*message.Message, error) {
	// Parse will submit the original message if it encounters an error
	var data map[string]interface{}
	err := json.Unmarshal(msg.GetContent(), &data)
	if err != nil {
		return msg, nil
	}

	// Step 2: Look for the attribute "ddtags" and read its value
	ddtagsData, exists := data["ddtags"]
	if !exists {
		return msg, nil
	}
	ddtagsString, ok := ddtagsData.(string)
	if !ok {
		return msg, nil
	}

	// Step 3: Remove the "ddtags" attribute from the map
	delete(data, "ddtags")

	// Step 4: Marshal the modified map back to JSON
	modifiedJSON, err := json.Marshal(data)
	if err != nil {
		return msg, nil
	}

	// Step 5: Convert the ddtagsSlice into a string array
	var ddtagsSlice []string
	if len(ddtagsString) > 0 {
		ddtagsSlice = strings.Split(ddtagsString, ",")
	}

	// append ddtags to message origin tags
	// set content to modifiedJSON
	if len(ddtagsSlice) > 0 {
		msg.SetTags(ddtagsSlice)
	}
	msg.SetContent(modifiedJSON)

	return msg, nil
}

// SupportsPartialLine implements Parser#SupportsPartialLine
func (p *integrationFileFormat) SupportsPartialLine() bool {
	return false
}
