# pkg/serializer/marshaler

## Metadata payload types are mirrored in the intake

`MetadataPayloadType` values in `payload_type.go` are sent in the `DD-Payload-Type` header and
are mirrored by an allowlist in the Datadog intake, which lives in a private Datadog repository
(dd-go, `apps/propjoe/agent_payload_type.go`). Never rename, remove, or reuse an existing value:
the list is append-only on both sides.

When a change adds a `MetadataPayloadType`:

- **External contributors** (no access to Datadog's private repositories): nothing to do on the
  intake side. Call out the new value in the PR description so reviewers can handle it.
- **Datadog contributors**: add the same string to the intake allowlist before, or together with,
  the Agent release that ships it, and link that change in the Agent PR description.
- **Reviewers**: do not merge a PR that adds a value until the intake change exists. For an
  external PR, the Datadog reviewer makes or requests that change.
