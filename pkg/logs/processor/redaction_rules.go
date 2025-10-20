// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"regexp"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

// defaultPIIRedactionRules are the built-in PII redaction patterns that are automatically
// applied when logs_config.auto_redact_pii is enabled.
//
// FUTURE THOUGHTS:
// - don't specify "*_REDACTED" type, use generic "REDACTED" for greater privacy (redaction type currently specified for development purposes)
// - make the default rules configurable through a config file?
var defaultPIIRedactionRules = []*config.ProcessingRule{
	{
		Type:               config.MaskSequences,
		Name:               "auto_redact_email",
		Pattern:            `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`,
		ReplacePlaceholder: "[EMAIL_REDACTED]",
		Regex:              regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),
		Placeholder:        []byte("[EMAIL_REDACTED]"),
	},
	{
		Type:               config.MaskSequences,
		Name:               "auto_redact_credit_card",
		Pattern:            `\b(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\d{3})\d{11})\b`,
		ReplacePlaceholder: "[CC_REDACTED]",
		Regex:              regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\d{3})\d{11})\b`),
		Placeholder:        []byte("[CC_REDACTED]"),
	},
	{
		Type:               config.MaskSequences,
		Name:               "auto_redact_ssn",
		Pattern:            `\b\d{3}-\d{2}-\d{4}\b`,
		ReplacePlaceholder: "[SSN_REDACTED]",
		Regex:              regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
		Placeholder:        []byte("[SSN_REDACTED]"),
	},
	{
		Type:               config.MaskSequences,
		Name:               "auto_redact_phone",
		Pattern:            `(?:\+?1[-.\s]?)?(?:\([0-9]{3}\)|[0-9]{3})[-.\s]?[0-9]{3}[-.\s]?[0-9]{4}`,
		ReplacePlaceholder: "[PHONE_REDACTED]",
		Regex:              regexp.MustCompile(`(?:\+?1[-.\s]?)?(?:\([0-9]{3}\)|[0-9]{3})[-.\s]?[0-9]{3}[-.\s]?[0-9]{4}`),
		Placeholder:        []byte("[PHONE_REDACTED]"),
	},
	{
		Type:               config.MaskSequences,
		Name:               "auto_redact_ipv4",
		Pattern:            `\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`,
		ReplacePlaceholder: "[IP_REDACTED]",
		Regex:              regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`),
		Placeholder:        []byte("[IP_REDACTED]"),
	},
}
