# RC Secrets workload preparation prototype

This prototype connects the x509 Agent client to delegated authentication. It
implements the merged `PrepareSecretDelivery` protocol and a staging-only
`DeliverPOCSecret` handler for the fixed `rc-secrets-poc: delivered` fixture.
Successful decryption produces an acknowledgment; it does not apply a secret to
Agent configuration. Production product authorization and retrieval remain
separate work.
The x509 client currently supports non-FIPS Linux and macOS builds.

## Flow

1. RC calls `SecretsService/PrepareSecretDelivery` over the existing x509 tunnel,
   with a product and an encryption context ID.
2. The Agent checks its exact product allowlist, then asks delegated authentication
   for a fresh `rc_secrets` workload token. Delegated authentication obtains the
   configured provider's identity proof and exchanges it with ETS. A failure
   returns `IDENTITY_UNAVAILABLE`, without a key or provider error details.
3. After that exchange succeeds, the Agent creates an X25519 key pair and returns
   the token, public key, and remaining key lifetime. It keeps the private key
   locally for at most one minute of usable lifetime.
4. RC must validate the token for the expected organization and RC Secrets grant,
   authorize the product and requested secrets, and encrypt the delivery with
   NaCl's anonymous sealed box format before the key expires.
5. A product handler calls `rcx509client.Component.OpenSecret(product, contextID,
   ciphertext)`. This consumes the context even if decryption fails. The product
   handler owns decoding and applying the plaintext; preparation is not delivery.

The workload token does not attest the encryption public key. The preparation
response depends on the authenticated tunnel for integrity. Reusing an unexpired
pending context returns its existing key without extending the deadline. There
are at most 128 pending contexts; shutdown clears them. Expired keys are unusable
and are removed on subsequent preparation/decryption operations or shutdown.

## Setup

Configure one AWS delegated-auth instance for the dedicated staging test org.
Supply temporary credentials for the mapped AWS role through the supported AWS
provider configuration, and set `delegated_auth.org_uuid` and the staging `site`.
The AWS account must be integrated into that org and its ordinary intake mapping
must match the caller's STS assumed-role ARN. Enable `rc-secrets-wif` for that org
only after the AWS RC grant is deployed in ETS. The POC treats every matching AWS
intake mapping in an enabled org as RC-authorized; no mapping permission field or
custom OIDC setup is required.

The end-to-end onboarding runbook is maintained in
[experimental](https://github.com/ddoghq-sandbox/experimental/tree/main/users/devin.andrews/rc-secrets-aws-poc).
This draft is based on an integration branch containing current main and the
existing x509/protocol prerequisites; it adds no OIDC implementation.

Enable the handler explicitly in an Agent build containing the x509 client:

```yaml
remote_configuration:
  enabled: true
  x509:
    enabled: true
    secrets:
      enabled: true
      products: [rc-secrets-poc]
```

The dummy handler requires both `site: datad0g.com` and `rc-secrets-poc` in the
product list. Other products may use the preparation helper with their own
handler. Empty product lists deny all preparations. The handler uses the
`remote_configuration.api_key` delegated-auth instance when that override is
configured, otherwise `api_key`. A static key or missing instance cannot substitute
for workload identity, and there is no fallback between those instances.

For this local demo, native logging is enabled when the dummy handler is
registered. Run with
`RC_LOG=warn,rc_x509_client::connection::handler::delegate::fsm::handshaking=debug`
and read the verified ID from `connection handshake complete`. The ID changes
on reconnect. Do not enable broad native debug logs: they may include protocol
payloads. The runbook supplies isolated runtime/config paths and the complete
backend invocation.

## Local checks

```sh
bazelisk test //comp/remote-config/rcx509client/impl:impl_test \
  //comp/core/delegatedauth/impl:impl_test --test_output=errors
bazelisk test //comp/remote-config/rcx509client/impl:impl_test \
  --@rules_go//go/config:race --test_output=errors
```

These tests cover the component wiring and preparation/decryption lifecycle with
an injected token provider. They do not establish that the deployed RC tunnel,
ETS, and product backend interoperate. A live staging round trip remains required.

## AWS staging smoke check

`TestAWSRCSecretsPOCSmoke` is skipped unless `RC_SECRETS_POC_ORG_UUID` is set.
Build its executable once, then run it in a dedicated process with temporary AWS
role credentials. It is deliberately fixed to `datad0g.com` and does not print
credentials. The runbook supplies `poc.py exec-role` to assume the role and pass
credentials directly to the child process (the Agent does not read AWS profiles).

```sh
bazelisk run --script_path=/tmp/rc-secrets-aws-smoke \
  //comp/remote-config/rcx509client/impl:impl_test \
  -- -test.run '^TestAWSRCSecretsPOCSmoke$' -test.v -test.timeout=2m
```

Run the generated script with `RC_SECRETS_POC_ORG_UUID`, `AWS_ACCESS_KEY_ID`,
`AWS_SECRET_ACCESS_KEY` and `AWS_SESSION_TOKEN` in its environment. Require exit
zero and `RC_AWS_POC_VERIFIED`. This exercises real intake bootstrap and AWS
workload exchange plus local preparation/decryption. The local peer does not
verify the ETS token, run Magic Tunnel, or establish RC/product authorization;
complete the separate backend and tunnel acceptance checks before calling it
an end-to-end secret delivery.
