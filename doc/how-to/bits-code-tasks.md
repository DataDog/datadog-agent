# Running tasks in Bits Code

Set `DD_BITS_CODE=1` when running Agent tasks in a Bits Code coding session.
This opt-in marker selects task authentication policy; it does not authenticate a
user, provide API tokens or cloud credentials, or mark the session as CI.

Local build metadata uses real reachable Git tags. If tags are absent, fetch the
required tags and history through the host coding agent's authenticated Git
operation, then retry. Bits mode does not substitute a fabricated release version.

`GithubAPI(public_repo=True)` uses PyGithub's unauthenticated client in this mode.
This supports public read operations, subject to GitHub's unauthenticated rate
limits. Private repository access and writes require the host coding agent's
existing authenticated GitHub operation; these Python tasks do not bridge to it.
Do not pass `public_repo=True` as a way to enable authenticated operations.

GitLab tasks use a tokenless client rather than rejecting API access in advance.
The client uses the existing `GITLAB_HOST` endpoint (HTTPS, with an optional port)
and the public CA bundle in `REQUESTS_CA_BUNDLE` or `SSL_CERT_FILE`. Without an
endpoint override it targets the normal GitLab host without credentials; the mode
marker does not create a proxy route. TLS verification remains enabled. Pagination
stays on the configured endpoint, and SDK follow-up calls use the project's path
while preserving numeric IDs in returned objects. Redirects are not followed.

The caller must make that endpoint reachable from the container and mount its
public CA bundle read-only. No token, `.netrc`, or credential directory is needed:
provided and ambient GitLab tokens are ignored in this mode. The proxy's normal
repository and operation policy still applies, and denied requests retain their
API errors without attempting a local login. This client does not enable extra
repositories, raw job logs or writes in Bits.

A GitHub-backed session does not automatically acquire access to its GitLab CI
mirror. The GitLab integration and a separately authorized proxy route must exist
before direct GitLab tasks can work. For CI investigation without such a route,
use the coding agent's available Datadog MCP tools.

Shared GitHub/GitLab token helpers, infrastructure authentication, `aws-vault`
environment export and `dd-auth` API/App key bootstrap fail before launching
credential commands. The same policy applies even if CI or token variables are
present. Perform the requested service operation through an available authenticated
host coding agent tool; when none exists, use an authenticated developer environment.
Do not copy host credentials into the session or set CI variables to bypass this policy.

This is bounded support for tasks using these shared helpers. Independent
cloud provisioning, E2E setup, release and deployment tasks with their own
workstation authentication commands are unsupported in Bits Code. The marker is
not a sandbox or a global subprocess filter; it does not make those workflows safe
to invoke. No generic credential broker or automatic host-tool dispatch is provided.

Outside Bits Code mode, existing developer and CI authentication behavior is unchanged.
