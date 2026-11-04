"""Shared values and macros across the agent product build."""

# The root for config files in the product payload.  This is an absolute path
# on Linux, and relative to {install_dir} on other platforms.
ETC_DIR_SELECTOR = {
    "@platforms//os:linux": "/etc/datadog-agent",
    "@platforms//os:macos": "etc",
    "//conditions:default": "etc/datadog-agent",
}

# The root for config files in the product payload.  This is an absolute path
# on Linux, and relative to {install_dir} on other platforms.
CONF_DIR_SELECTOR = {
    "@platforms//os:linux": "/etc/datadog-agent/conf.d",
    "@platforms//os:macos": "etc/conf.d",
    "//conditions:default": "etc/datadog-agent/conf.d",
}

def _join(base, suffix):
    return base + "/" + suffix if suffix else base

def etc_dir(suffix = ""):
    """Returns a select() of the config root for the target platform.

    Args:
      suffix: optional path to append to the config root.
    """
    return select({k: _join(v, suffix) for k, v in ETC_DIR_SELECTOR.items()})

def conf_dir(suffix = ""):
    """Returns a select() of the conf.d root for the target platform.

    Args:
      suffix: optional path to append to the conf.d root.
    """
    return select({k: _join(v, suffix) for k, v in CONF_DIR_SELECTOR.items()})
