"""Determine compression level for tar files.

This is an abstraction to hide the logic of how we choose the compression level.
What exists now is a mix of choices, depending on if you are doing a a developer
build vs. a CI run, vs. a deployment.
"""

load("@agent_volatile//:env_vars.bzl", "env_vars")

COMPRESSION_HIGH = 9
COMPRESSION_MEDIUM = 5

# Due to a bug in pkg_tar, setting compression to 0 results in using
# the default, which is 6.
# https://github.com/bazelbuild/rules_pkg/issues/1048
COMPRESSION_OFF = 1

def get_compression_level():
    """Returns the compression level we should use for archives.

    The logic essentially mimics what we are currently doing in omnibus builds.

      if ENV['FORCED_PACKAGE_COMPRESSION_LEVEL']
        # This is only used for armhf (32 bit) builds right now.
        COMPRESSION_LEVEL = int(ENV['FORCED_PACKAGE_COMPRESSION_LEVEL'])
      elif ENV["DEPLOY_AGENT"] == "true"
        COMPRESSION_LEVEL = 9
      else
        COMPRESSION_LEVEL = 5

    TODO: improve this so developer (desktop) builds skip compression entirely and non-release
    CI builds make a choice that balances compress time vs. artifact upload speed.

    Returns:
        int
    """
    if env_vars.FORCED_PACKAGE_COMPRESSION_LEVEL:
        compression_level = int(env_vars.FORCED_PACKAGE_COMPRESSION_LEVEL)
        if compression_level >= 0:
            return compression_level if compression_level > 0 else COMPRESSION_OFF

    if env_vars.DEPLOY_AGENT:
        if env_vars.DEPLOY_AGENT == "true":
            return COMPRESSION_HIGH

    return COMPRESSION_MEDIUM
