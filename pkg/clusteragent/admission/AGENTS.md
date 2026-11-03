# Admission controller mutation guidance

## Unknown Pod fields and list ordering

`mutate/common.Mutate` generates a JSON Patch by comparing serialized typed
`corev1.Pod` snapshots before and after normalization and mutation. The API server
applies that patch to the original raw Pod. Fields unknown to the Kubernetes client
are absent from both snapshots, so their absence alone does not generate removals.
This covers fields from newer Kubernetes versions as well as custom Kubernetes forks.

Init containers are reconciled separately by name. List changes use explicit
insertions, moves, and removals, followed by typed field diffs for matched
containers at their final indexes. This preserves the original raw elements,
including unknown fields, when APM SSI or other webhooks prepend init containers.
The init-container baseline is deep-copied before normalization and mutation so
in-place changes remain visible in the field diff.

This is not a general guarantee of unknown-field preservation. Other lists,
including nested lists within init containers, still use a positional typed diff.
Prepending, inserting, removing, or reordering their elements can leave unknown
fields on the wrong element or discard them. Removing or replacing an enclosing
object can also discard unknown fields.

When changing list mutations:

- Do not assume a typed-before/after diff preserves unknown fields when element
  indexes change. This applies to nested lists such as environment variables and
  volume mounts, as well as container lists.
- Preserve required execution and dependency ordering; do not switch a prepend to
  an append solely to avoid this limitation.
- For affected regression cases, apply the returned patch to the original raw JSON
  and verify unknown fields remain on the same logical element. Decoding the result
  only into `corev1.Pod` cannot detect their loss or reassignment.
- Changes requiring preservation across other list edits need patch generation
  that preserves element identity; init-container name matching does not extend
  to their nested lists.
