// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

// AutoRedactConfig holds configuration for PII auto-redaction.
// All fields are pointers to allow distinguishing between "not set" and explicitly set values.
// This enables per-source overrides of global settings.
type AutoRedactConfig struct {
	// Enabled is the parent switch for PII auto-redaction
	Enabled *bool `mapstructure:"enabled" json:"enabled" yaml:"enabled"`
	// Email controls email redaction
	Email *bool `mapstructure:"email" json:"email" yaml:"email"`
	// CreditCard controls credit card redaction
	CreditCard *bool `mapstructure:"credit_card" json:"credit_card" yaml:"credit_card"`
	// SSN controls SSN redaction
	SSN *bool `mapstructure:"ssn" json:"ssn" yaml:"ssn"`
	// Phone controls phone number redaction
	Phone *bool `mapstructure:"phone" json:"phone" yaml:"phone"`
	// IP controls IP address redaction
	IP *bool `mapstructure:"ip" json:"ip" yaml:"ip"`
}
