# admission-webhook-no-silent-nil-cert — Admission webhook never serves a nil cert while swallowing the error

**Type:** Safety · **Assertion:** `AlwaysOrUnreachable` · **Priority:** P2 · **Intent:** known-defect-reproducer

**Provenance:** merged from 1 discovery agent(s): admission-webhook-serves-nil-cert-silently

## Property

The admission-controller HTTPS server never presents a nil certificate while swallowing the fetch error; every TLS handshake either uses a valid cert or fails loudly with a surfaced error.


## Invariant / assertion

`assert.AlwaysOrUnreachable`: the GetCertificate callback never returns (nil,nil). AlwaysOrUnreachable fits — the callback runs per handshake (optional, admission must be enabled), but any invocation must not silently return a nil cert. Today it logs and returns (nil,nil) on error (server.go:141-144), so this is expected to expose the silent-nil path.


## Paired witness (R1 — proves the hazardous window was scheduled)

`Reachable`: the GetCertificate error branch executed (lister miss / frozen informer).


## Antithesis angle

Cert is fetched per-handshake from a secrets Lister; on error it logs and returns (nil,nil) → handshake proceeds with nil cert, failing opaquely. If the informer cache is stale/unsynced (partition freezes it, informer_client_timeout=0), the served cert can be the old one after rotation, or nil. Partition DCA<->apiserver to freeze the secret informer during a cert fetch.


## Why it matters

A silent nil cert yields opaque TLS handshake failures that are hard to diagnose; combined with failurePolicy=Fail it can block pod creation cluster-wide with a misleading error. Health/observability accuracy property.


## Mechanism refinement (from open-question investigation)

Scope clarification, no invalidation. The (nil,nil) path DOES fail the TLS handshake loudly (errNoCertificates) rather than serving a nil/empty cert — so 'serves a nil certificate' is inaccurate at the transport level; the accurate defect is that the ORIGINAL fetch error is swallowed (logged only, server.go:142-144) making the resulting handshake failure opaque/unattributable to the apiserver. The assertion 'GetCertificate never returns (nil,nil)' remains valid and worth testing.


## Fault dependencies

- network partition between DCA and apiserver (freezes secret informer; enabled by default)
- requires admission_controller.enabled + leader


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.AlwaysOrUnreachable(cert != nil || err != nil)` in GetCertificate; a `Reachable` on the error branch confirms the stale/failed-fetch path is exercised.


## Open questions (post-investigation)

- Default admission_controller.failure_policy actually rendered by the Helm chart / Operator in the harness (code default is 'Ignore', common_settings.go:653; the shipped/harness-rendered value lives in external repos and is not pinned in antithesis/scratchbook/deployment-topology.md). Governs severity only. `(needs human input)`


### Investigation Log

#### Q1/Q5: crypto/tls behavior when GetCertificate returns (nil,nil) with no static Certificates; does it fail the handshake or fall back?

Examined Go 1.26 src/crypto/tls/common.go getCertificate (:1317-1328). With GetCertificate returning (nil,nil), the guard `cert != nil || err != nil` (:1321) is false so it falls through; len(Certificates)==0 (admission server sets only GetCertificate, server.go:136-147) → returns errNoCertificates ("tls: no certificates configured", :1327). Concluded: hard handshake failure at the TLS layer, no panic, no empty/wrong cert, no fallback. RESOLVED (both Q1 and Q5).

#### Q3: Does GetCertificateFromLister return stale-but-valid vs strictly error when Secret missing?

Examined pkg/util/kubernetes/certificate/certificate.go:139-151. lister.Get reads the informer CACHE; if the Secret is present (even stale) it parses & returns it (stale-but-valid); it errors ONLY when the Secret is absent from the cache or ParseSecretData fails. Concluded: stale-but-valid masking is possible; strict error only on cache-miss/parse-fail. RESOLVED.

#### Q4: Frequency of cert rotation relative to leadership churn

Examined common_settings.go:636-637: certificate.validity_bound = 365*24h (1yr), expiration_threshold = 30*24h (refresh 1mo before expiry). Concluded: rotation is ~yearly; leadership churn is far more frequent, so the leader-gated resync gap during rotation is rarely hit. RESOLVED.


---

## Source discovery evidence (raw, per contributing agent)


### from `admission-webhook-serves-nil-cert-silently`

## Claim
The admission webhook's TLS `GetCertificate` callback returns `(nil, nil)` when the cert Secret cannot be fetched, causing a silent TLS handshake failure and, via failurePolicy, a silently-unenforced admission webhook.

## Mechanism (verified in source)
```go
// cmd/cluster-agent/admission/server.go:137-145
GetCertificate: func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
    secretNs := namespace.GetResourcesNamespace()
    secretName := pkgconfigsetup.Datadog().GetString("admission_controller.certificate.secret_name")
    cert, err := certificate.GetCertificateFromLister(s.secretsLister.Secrets(secretNs), secretName)
    if err != nil {
        log.Errorf("Couldn't fetch certificate: %v", err)  // logged, then swallowed
    }
    return cert, nil  // cert may be nil; err discarded
}
```
When `err != nil`, `cert` is nil and the callback returns `(nil, nil)`. Per crypto/tls, a GetCertificate result of nil-cert/nil-error yields no usable certificate for the handshake, so the handshake fails (from the apiserver's perspective the webhook endpoint is unreachable/broken).

## Downstream effect (from SUT analysis §7, to be tested)
- `admission_controller.failure_policy` defaults to `Ignore` (unknown value also → Ignore): apiserver proceeds and admits the pod **un-mutated** — the intended security/config mutation is silently dropped.
- With `Fail`: all matching pod creation is blocked cluster-wide.
- The cert fetch is per-handshake from a possibly-stale informer lister; cert rotation is leader-gated and can be missed during leadership churn (SUT §7 item 5).

## Failure scenario
1. DCA leader is admission controller; cert stored in a Secret, read via informer lister.
2. Antithesis partitions the DCA from the apiserver; with `informer_client_timeout=0` the watch has no client-side timeout and the informer cache freezes without surfacing an error (SUT §7).
3. The Secret is rotated/created (or was never synced after a leadership flap), so `GetCertificateFromLister` returns an error.
4. GetCertificate logs and returns `(nil,nil)`; the apiserver→webhook TLS handshake fails.
5. With default failurePolicy=Ignore, every pod during the window is admitted without the webhook's mutation — a silent security-control bypass with no rejection and only a log line.

## Key observations
- The error is observable only as a log line; there is no metric/assertion and no fail-fast. Returning the error (instead of nil) would at least make the handshake failure attributable, but the mutation would still be skipped under Ignore — so the real fix is operational awareness plus failurePolicy choice.
- This is distinct from a bypass of authentication; it is a fail-open of a mutation/validation control under a fault window.

## Timing window
Duration of the secret-informer staleness — potentially unbounded under a silent watch freeze (no client-side timeout), or the gap between a leadership flap and the leader-gated cert resync.
