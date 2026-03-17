"""Determine compression level for tar files.

This is an abstraction to hide the logic of how we choose the compression level.
What exists now is a mix of choices, depending on if you are doing a a developer
build vs. a CI run, vs. a deployment.
"""

load("@agent_volatile//:env_vars.bzl", "env_vars")

COMPRESSION_HIGH = 9

# Due to a bug in pkg_tar, setting compression to 0 results in using
# the default, which is 6.
# https://github.com/bazelbuild/rules_pkg/issues/1048
COMPRESSION_OFF = 1

def get_compression_level():
    """Returns the compression level we should use for archives.

    The logic is
        - If FORCED_PACKAGE_COMPRESSION_LEVEL is set, use that.
        - For developer builds, do not (or use mimimal) compress.
        - Otherwise, trigger off --config=release (which should always be
          used in CI) to set to high compression (9)

    This is different than the omnibus logic, which is essentially
      if ENV['FORCED_PACKAGE_COMPRESSION_LEVEL']
        # This is only used for armhf (32 bit) builds right now.
        COMPRESSION_LEVEL = int(ENV['FORCED_PACKAGE_COMPRESSION_LEVEL'])
      elif ENV["DEPLOY_AGENT"] == "true"
        # DEPLOY_AGENT is currently set as the default for omnibus CI, and
        # can also be set by by using --deploy on the invoke task. In the
        # top level project omnibus scripts, it triggers compression.
        # It might have other meanings which are irrelevant to compression.
        COMPRESSION_LEVEL = 9
      else
        COMPRESSION_LEVEL = 5

    Returns:
        Int from 1 to 9.
    """
    if env_vars.FORCED_PACKAGE_COMPRESSION_LEVEL:
        compression_level = int(env_vars.FORCED_PACKAGE_COMPRESSION_LEVEL)
        if compression_level > 0:
            return compression_level

    return select({
        "//:is_release": COMPRESSION_HIGH,
        "//conditions:default": COMPRESSION_OFF,
    })
