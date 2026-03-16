"""Determine compression level for tar files.

This is an abstraction to hide the logic of how we choose the compression level.
What exists now is a mix of choices, depending on if you are doing a a developer
build vs. a CI run, vs. a deployment.
"""

load("@agent_volatile//:env_vars.bzl", "env_vars")

def get_compression_level():
    """Returns the compression level we should use for archives.

    The logic is
        - For developer builds, do not (or use mimimal) compress.
        - Otherwise, trigger off off --config=release, which should always be
          used in CI.
          - If FORCED_PACKAGE_COMPRESSION_LEVEL is set, use that.
          - Otherwise set to 9

    This is different than the omnibus logic, which is essentially
      if ENV['FORCED_PACKAGE_COMPRESSION_LEVEL']
        # This is only used for armhf (32 bit) builds right now.
        COMPRESSION_LEVEL = int(ENV['FORCED_PACKAGE_COMPRESSION_LEVEL'])
      elif ENV["DEPLOY_AGENT"] == "true"
        # DEPLOY_AGENT is mostly true. It's not clear to me yet if "deploy" means to
        # build the release version, or simply to include the agent binary. It is only
        # set false for installer and integrations_core, so it seems the second, even if
        # the word deploy generally is associated with release.
        COMPRESSION_LEVEL = 9
      else
        COMPRESSION_LEVEL = 5

    Returns:
        Int from 1 to 9.
    """
    if env_vars.FORCED_PACKAGE_COMPRESSION_LEVEL:
        compression_level = int(env_vars.FORCED_PACKAGE_COMPRESSION_LEVEL)
    else:
        compression_level = 9
    return select({
        "//:is_release": compression_level,
        # Due to a bug in pkg_tar, setting compression to 0 results in using
        # the default, which is 6.
        # https://github.com/bazelbuild/rules_pkg/issues/1048
        "//conditions:default": 1,
    })
