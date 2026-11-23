# pkg/serializer/marshaler

## Metadata payload types must be known to the intake

`MetadataPayloadType` values in `payload_type.go` are sent in the `DD-Payload-Type` header. The
Datadog intake keeps its own list of the values it recognizes, maintained outside this repository.
That list was introduced in ddoghq/dd-go#32163 (Datadog-internal).

- The list is append-only on both sides: never rename, remove, or reuse an existing value.
- A new value must be registered with the intake before, or together with, the Agent release that
  sends it.

When a change adds a `MetadataPayloadType`:

- **External contributors**: nothing to do on the intake side. Call out the new value in the PR
  description so reviewers can handle it.
- **Datadog contributors**: register the value with the intake and link that change in the Agent
  PR description.
- **Reviewers**: do not merge a PR that adds a value until the intake change exists. For an
  external PR, the Datadog reviewer makes or requests that change.
