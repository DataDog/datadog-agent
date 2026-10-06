// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package enrollment

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/regions"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// Pending credentials live BESIDE the identity, not in the outcome journal.
// Prepare (key+name) is durable before POST; committed adds the returned URN.
// A prepare record alone is not proof that the backend did/did not commit.
type pendingIdentity struct {
	AttemptID  string `json:"attempt_id"`
	ConfigHash string `json:"config_hash"`
	RunnerName string `json:"runner_name,omitempty"`
	PersistedIdentity
}

func pendingIdentityPath(cfg model.Reader, id string) string {
	return cfg.GetString(setup.PARIdentityFilePath) + ".pending-" + id
}

// PhoneHomeConfigHash binds discovery and POST to the same credential/config.
// Only the digest is written, in protected pending identity storage. It is never
// logged or included in the outcome journal. Rotation requires joint restart.
func PhoneHomeConfigHash(cfg model.Reader, hostname string) string {
	values := []interface{}{hostname, cfg.GetString("api_key"), cfg.GetString("site"), cfg.GetString("dd_url"),
		cfg.GetBool(setup.PAREnabled), cfg.GetBool(setup.PARSelfEnroll), cfg.GetBool(setup.PARApiKeyOnlyEnrollment),
		cfg.GetProxies(),
		cfg.GetBool("skip_ssl_validation"), cfg.GetString("min_tls_version"), cfg.GetStringMapString(setup.PAROpmsExtraHeaders),
		os.Getenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS")}
	data, _ := json.Marshal(values)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readPendingIdentity(cfg model.Reader, a *PhoneHomeAttempt) (*pendingIdentity, error) {
	var pending pendingIdentity
	if err := readPhoneHomeJSON(pendingIdentityPath(cfg, a.ID), &pending); err != nil {
		return nil, err
	}
	if pending.AttemptID != a.ID {
		return nil, ErrStalePhoneHomeAttempt
	}
	return &pending, nil
}

// PendingPersistence keeps the helper alive until both the identity and its
// recovery outcome are durable. Error/status never formats credential fields.
type PendingPersistence struct {
	pending *pendingIdentity
	// SafeToExit requires durable key/URN AND a published recovery outcome.
	// Credentials alone cannot tell core to schedule a storage-only retry.
	SafeToExit bool
}

func (e *PendingPersistence) Error() string { return "enrollment identity persistence pending" }

// Retain writes the returned identity to protected storage only. The resident
// executor retries this with no POST. It can then exit and let core schedule a
// bounded, idempotent publication retry via the existing executor startup path.
func (e *PendingPersistence) Retain(cfg model.Reader) error {
	if err := writePhoneHomeJSON(pendingIdentityPath(cfg, e.pending.AttemptID), e.pending); err != nil {
		return err
	}
	return RecordPhoneHomeOutcome(cfg, PhoneHomeOutcome{AttemptID: e.pending.AttemptID, EnrollmentFailure: opms.EnrollmentFailure{Category: "persistence_pending"}, PersistencePending: true})
}

func pendingResult(p *pendingIdentity) (*Result, error) {
	key, err := util.Base64ToJWK(p.PrivateKey)
	if err != nil {
		return nil, errors.New("invalid pending private key")
	}
	private, ok := key.Key.(*ecdsa.PrivateKey)
	if !ok || !key.Valid() {
		return nil, errors.New("invalid pending private key")
	}
	if _, err := util.ParseRunnerURN(p.URN); err != nil {
		return nil, errors.New("invalid pending runner identity")
	}
	return &Result{PrivateKey: private, URN: p.URN, RunnerName: p.RunnerName, Hostname: p.Hostname, APIKeyHash: p.APIKeyHash}, nil
}

func publishPendingIdentity(ctx context.Context, cfg model.Reader, p *pendingIdentity) (*Result, error) {
	result, err := pendingResult(p)
	if err != nil {
		return nil, err
	}
	if err := PersistIdentity(ctx, cfg, result); err != nil {
		outcomeErr := RecordPhoneHomeOutcome(cfg, PhoneHomeOutcome{AttemptID: p.AttemptID, EnrollmentFailure: opms.EnrollmentFailure{Category: "persistence_pending"}, PersistencePending: true})
		return nil, &PendingPersistence{pending: p, SafeToExit: outcomeErr == nil}
	}
	if err := RecordPhoneHomeOutcome(cfg, PhoneHomeOutcome{AttemptID: p.AttemptID, EnrollmentFailure: opms.EnrollmentFailure{Category: "identity_persisted"}}); err != nil {
		return nil, &PendingPersistence{pending: p}
	}
	return result, nil
}

// RecoverPhoneHomeIdentity runs before reading the active identity, which may
// itself be unreadable after a failed publication. It never submits a POST.
func RecoverPhoneHomeIdentity(ctx context.Context, cfg model.Reader, hostname string) error {
	a, err := ReadPhoneHomeAttempt(cfg)
	if err != nil || a == nil {
		return err
	}
	p, err := readPendingIdentity(cfg, a)
	if err != nil {
		return err
	}
	if p.ConfigHash != PhoneHomeConfigHash(cfg, hostname) {
		failure := &opms.EnrollmentFailure{Category: "config_mismatch"}
		_ = RecordPhoneHomeOutcome(cfg, PhoneHomeOutcome{AttemptID: a.ID, EnrollmentFailure: *failure})
		return failure
	}
	if p.URN == "" {
		return nil
	}
	_, err = publishPendingIdentity(ctx, cfg, p)
	return err
}

// enrollPhoneHome uses the existing Go OPMS client and identity publisher. No
// additional backend recovery API or idempotency guarantee is assumed.
func enrollPhoneHome(ctx context.Context, cfg model.Reader, identifier *AgentIdentifier) (_ *Result, retErr error) {
	a, err := ReadPhoneHomeAttempt(cfg)
	if err != nil || a == nil {
		return nil, errors.New("phone-home launch authorization missing")
	}
	p, err := readPendingIdentity(cfg, a)
	if err != nil {
		return nil, errors.New("phone-home pending storage unreadable")
	}
	if p.ConfigHash != PhoneHomeConfigHash(cfg, identifier.Hostname) {
		failure := &opms.EnrollmentFailure{Category: "config_mismatch"}
		_ = RecordPhoneHomeOutcome(cfg, PhoneHomeOutcome{AttemptID: a.ID, EnrollmentFailure: *failure})
		return nil, failure
	}
	if p.URN != "" {
		return publishPendingIdentity(ctx, cfg, p)
	}
	if err := claimPhoneHomePOST(cfg, a); err != nil {
		return nil, err
	}
	defer func() {
		if retErr == nil {
			return
		}
		var pending *PendingPersistence
		if errors.As(retErr, &pending) {
			return
		} // already published a precise outcome
		failure := &opms.EnrollmentFailure{Category: "local_config_or_storage"}
		var typed *opms.EnrollmentFailure
		if errors.As(retErr, &typed) {
			failure = typed
		}
		_ = RecordPhoneHomeOutcome(cfg, PhoneHomeOutcome{AttemptID: a.ID, EnrollmentFailure: *failure})
	}()
	if !cfg.GetBool(setup.PAREnabled) || !cfg.GetBool(setup.PARSelfEnroll) || !cfg.GetBool(setup.PARApiKeyOnlyEnrollment) ||
		flavor.GetFlavor() != flavor.DefaultAgent || len(cfg.GetStringMapString(setup.PAROpmsExtraHeaders)) != 0 {
		return nil, &opms.EnrollmentFailure{Category: "invalid_config"}
	}
	private, public, err := util.GenerateKeys()
	if err != nil {
		return nil, err
	}
	encoded, err := private.MarshalJSON()
	if err != nil {
		return nil, err
	}
	p.PrivateKey = base64.RawURLEncoding.EncodeToString(encoded)
	p.Hostname, p.APIKeyHash = identifier.Hostname, HashAPIKey(cfg.GetString("api_key"))
	p.RunnerName = identifier.Hostname + "-" + a.ID
	// Persist key/name BEFORE mutation. A crash or lost response preserves them,
	// but never permits another POST just because a duplicate name could be used.
	if err := writePhoneHomeJSON(pendingIdentityPath(cfg, a.ID), p); err != nil {
		return nil, err
	}
	site := configutils.ExtractSiteFromURL(configutils.GetMainEndpoint(cfg, "https://api.", "dd_url"))
	if site == "" {
		site = "datadoghq.com"
	}
	client := opms.NewPublicClient(cfg, enrollmentBaseURL(cfg, site), nil)
	response, err := client.EnrollWithApiKeyOnly(ctx, cfg.GetString("api_key"), p.RunnerName, []modes.Mode{modes.ModePull}, public, identifier.Hostname, "", flavor.DefaultAgent)
	if err != nil {
		return nil, err
	}
	p.URN = util.MakeRunnerURN(regions.GetRegionFromDDSite(site), response.OrgID, response.RunnerID)
	if err := writePhoneHomeJSON(pendingIdentityPath(cfg, a.ID), p); err != nil {
		_ = RecordPhoneHomeOutcome(cfg, PhoneHomeOutcome{AttemptID: a.ID, EnrollmentFailure: opms.EnrollmentFailure{Category: "persistence_memory"}, PersistencePending: true, HelperRetained: true})
		return nil, &PendingPersistence{pending: p}
	}
	return publishPendingIdentity(ctx, cfg, p)
}

// CompletePhoneHomeAttempt retires the journal only after active identity has
// been confirmed. Pending credentials are no longer needed; no arbitrary paths
// from the journal are accepted. Old attempt directories cannot affect a new ID.
func CompletePhoneHomeAttempt(cfg model.Reader, a *PhoneHomeAttempt) error {
	root := PhoneHomeAttemptPath(cfg)
	if err := os.Rename(root, root+".complete-"+a.ID); err != nil {
		return err
	}
	if err := os.Remove(pendingIdentityPath(cfg, a.ID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncPhoneHomeDirectory(filepath.Dir(root))
}
