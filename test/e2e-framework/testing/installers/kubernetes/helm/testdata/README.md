# Managed receiver chart fixture

`datadog-3.245.2.tgz` is the unmodified Apache-2.0 Datadog Helm chart,
from https://github.com/DataDog/helm-charts/releases/download/datadog-3.245.2/datadog-3.245.2.tgz.
SHA-256: `7722489c353711d7c4034ea0d8f20d3b6d58a4730669e4235393c1ab77c336b7`.

It is retained here for offline manifest/route precedence tests, not installation.
Explicit installs download this same pinned chart and validate before mutations.
Legacy installs retain their previous chart selection policy.
