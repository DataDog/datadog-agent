"""Agent version string for the par-control binary.

par-control reports this to OPMS in the `X-Datadog-OnPrem-Version` header, which
OPMS uses to reason about runner capabilities. It must be the *agent* version —
the same thing the Go runner reports as `pkg/version.AgentVersion` — not the
crate version.

Deliberately scoped to this crate for now. The same PACKAGE_VERSION-or-milestone
logic also exists in //bazel/rules/go:go_binary.bzl (as x_defs, private) and in
//packages/rules:package_naming.bzl (with '-' -> '~' substitution for OS package
names). Those are load-bearing for every agent binary, so this crate keeps its own
copy rather than refactoring shared rules under deadline.

TODO(ACTP): once par-control has landed, factor the three copies into one shared
helper so a Go binary and a Rust binary can never report different versions.
"""

load("@agent_volatile//:env_vars.bzl", "env_vars")
load("@dd_release_json//:release_json.bzl", "release_json")

def _url_safe_to_standard(url_safe):
    """Convert a URL-safe agent version string to standard SemVer form.

    PACKAGE_VERSION is produced by `dda inv agent.version --url-safe`, which
    replaces the SemVer '+' build-metadata separator with '.'. AgentVersion
    expects the standard form with '+'.

    Examples:
      "7.83.0-devel.git.635.e3326d4.pipeline.1" -> "7.83.0-devel+git.635.e3326d4.pipeline.1"
      "7.83.0-rc.1.git.635.e3326d4"             -> "7.83.0-rc.1+git.635.e3326d4"
      "7.83.0"                                   -> "7.83.0"  (clean release, unchanged)

    Args:
      url_safe: the URL-safe dotted version string.

    Returns:
      The same version in standard SemVer form.
    """
    idx = url_safe.find(".git.")
    if idx < 0:
        return url_safe
    return url_safe[:idx] + "+git." + url_safe[idx + 5:]

def par_control_agent_version():
    """Return the agent version par-control should report to OPMS.

    Uses PACKAGE_VERSION from the environment when available (CI), otherwise falls
    back to release.json's current_milestone with a "-localbuild" suffix, matching
    the convention used by the Go binaries and by package naming.

    Returns:
      The agent version in standard SemVer form, e.g. "7.83.0-localbuild".
    """
    if env_vars.PACKAGE_VERSION:
        return _url_safe_to_standard(env_vars.PACKAGE_VERSION)
    return release_json.get("current_milestone") + "-localbuild"
