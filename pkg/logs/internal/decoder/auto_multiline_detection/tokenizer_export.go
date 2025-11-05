// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automultilinedetection

import "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"

// TokenizeBytes is a public wrapper around the internal tokenize method.
// This is exported for use by the processor package for PII detection.
func (t *Tokenizer) TokenizeBytes(input []byte) ([]tokens.Token, []int) {
	return t.tokenize(input)
}
