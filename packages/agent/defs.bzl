"""Shared values and macros across the agent product build."""

# The root for config files in the product payload.  This is an absolute path,
# not under {install_dir}.
ETC_DIR_SELECTOR = {
    "@platforms//os:macos": "etc",
    "//conditions:default": "etc/datadog-agent",
}

# The root for config files in the product payload.  This is an absolute path,
# not under {install_dir}.
CONF_DIR_SELECTOR = {
    "@platforms//os:macos": "etc/conf.d",
    "//conditions:default": "etc/datadog-agent/conf.d",
}
