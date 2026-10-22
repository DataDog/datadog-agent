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
