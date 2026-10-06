// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package enrollment

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
)

// PhoneHomePOC is deliberately not a rollout setting. It must be inherited by
// core and the executor in the isolated, matching-config Linux deployment.
func PhoneHomePOC() bool { return os.Getenv(app.PhoneHomePOCEnvVar) == "true" }

// PhoneHomeAttemptPath contains no credentials. Its existence is a durable
// launch latch: core restarts must not retry an ambiguous enrollment mutation.
// The POC requires an explicit absolute identity_file_path shared by both Go processes.
func PhoneHomeAttemptPath(cfg model.Reader) string {
	return cfg.GetString(setup.PARIdentityFilePath) + ".phone-home"
}

// ClaimPhoneHomePOST permits at most one enrollment POST per operator-authorized
// attempt, including across executor crashes. Only core creates the directory.
// ponytail: failures require operator recovery until enrollment supports idempotent retries.
func ClaimPhoneHomePOST(cfg model.Reader) error {
	identityPath := cfg.GetString(setup.PARIdentityFilePath)
	if !filepath.IsAbs(identityPath) {
		return errors.New("phone-home POC requires an absolute identity_file_path")
	}
	f, err := os.OpenFile(filepath.Join(PhoneHomeAttemptPath(cfg), "post"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("phone-home enrollment not authorized or already attempted; inspect the attempt journal")
	}
	// Persist the guard before sending anything. Never remove it on error.
	err = f.Sync()
	return errors.Join(err, f.Close())
}

// RecordPhoneHomeFailure stores only a fixed reason, never an HTTP body or error
// string. A missing/partial outcome is treated as ambiguous by core.
func RecordPhoneHomeFailure(cfg model.Reader, reason string) error {
	f, err := os.OpenFile(filepath.Join(PhoneHomeAttemptPath(cfg), "outcome"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return nil // Keep the original failure if another executor starts.
	}
	if err != nil {
		return err
	}
	_, err = f.WriteString(reason)
	return errors.Join(err, f.Close())
}
