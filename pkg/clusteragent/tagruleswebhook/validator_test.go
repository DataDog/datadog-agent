// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package tagruleswebhook

import (
	"crypto/tls"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/tagrules"
)

// Test partitions:
// - certificate generation: self-signed pair parses as a TLS key pair
// - validateRule: source kind and source expressions

// TestSelfSignedCertIsUsable covers: the generated self-signed pair parses as
// a serving certificate and the CA bundle decodes back to the certificate.
func TestSelfSignedCertIsUsable(t *testing.T) {
	certPEM, keyPEM, caBundle, err := selfSignedCert()
	if err != nil {
		t.Fatalf("selfSignedCert: %v", err)
	}
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	if decoded, err := base64.StdEncoding.DecodeString(caBundle); err != nil {
		t.Fatalf("decoding ca bundle: %v", err)
	} else if string(decoded) != string(certPEM) {
		t.Errorf("ca bundle does not round-trip to the certificate PEM")
	}
}

// TestValidateRuleSource covers: a source without a kind is rejected; source
// name/namespace expressions are compile-checked.
func TestValidateRuleSource(t *testing.T) {
	compiler, err := newCelCompiler()
	if err != nil {
		t.Fatalf("newCelCompiler: %v", err)
	}

	emptyKind := boolRule("ben-bitdiddle", "is_leader")
	emptyKind.Spec.Source = &tagrules.SourceRef{Kind: ""}
	if problems := validateRule(emptyKind, compiler); len(problems) == 0 {
		t.Errorf("source without kind: no problems reported, want one")
	}

	badName := boolRule("ben-bitdiddle", "is_leader")
	badName.Spec.Source = &tagrules.SourceRef{Kind: "Lease", Name: "entity.metadata."}
	problems := validateRule(badName, compiler)
	if len(problems) == 0 {
		t.Fatalf("source with bad name expression: no problems reported, want one")
	}
	if want := "source.name"; !contains(problems, want) {
		t.Errorf("source name problems: got %v, want an entry mentioning %q", problems, want)
	}
}

// TestValidateOwnership covers: the rule's own CR is excluded from the
// uniqueness check; another CR with the same tag key is reported.
func TestValidateOwnership(t *testing.T) {
	rule := boolRule("ben-bitdiddle", "is_leader")
	claimed := map[string]string{
		"ben-bitdiddle": "is_leader", // own entry: ignored
		"eva-lu-ator":   "is_leader", // conflicting claim
	}
	problems := validateOwnership(rule, nil, claimed)
	if len(problems) != 1 {
		t.Fatalf("ownership problems: got %v, want exactly one", problems)
	}
	if want := "eva-lu-ator"; !contains(problems, want) {
		t.Errorf("ownership problem: got %v, want it to name %q", problems, want)
	}

	// A distinct tag key on another rule is fine.
	claimed = map[string]string{"eva-lu-ator": "owning_team"}
	if problems := validateOwnership(rule, nil, claimed); len(problems) != 0 {
		t.Errorf("distinct tag key: got %v, want no problems", problems)
	}
}

// contains reports whether any message mentions substr.
func contains(messages []string, substr string) bool {
	for _, message := range messages {
		if strings.Contains(message, substr) {
			return true
		}
	}
	return false
}
