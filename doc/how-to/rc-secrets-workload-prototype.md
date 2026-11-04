# RC Secrets workload preparation prototype

This prototype connects the x509 Agent client to delegated authentication. It
implements the merged `PrepareSecretDelivery` protocol; product-specific delivery
handlers and the RC backend's secret retrieval/authorization remain separate work.
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

Configure and initialize delegated authentication using the
[OIDC workload identity guide](oidc-workload-identity.md). The prototype issuer
requires the `custom_oidc_auth` and `rc-secrets-wif` org flags and a matching
custom OIDC mapping with `rc_secrets_enabled: true`. Use claim-based org routing
until explicit destination routing is deployed in ETS.

Enable the handler explicitly in an Agent build containing the x509 client:

```yaml
remote_configuration:
  enabled: true
  x509:
    enabled: true
    secrets:
      enabled: true
      products: [example-product]
```

The product name above is illustrative; configure the name used by your product
handler. Empty product lists deny all preparations. The handler uses the
`remote_configuration.api_key` delegated-auth instance when that override is
configured, otherwise `api_key`. A static key or missing instance cannot substitute
for workload identity, and there is no fallback between those instances.

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
