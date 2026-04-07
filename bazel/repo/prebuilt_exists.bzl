"""Config settig to see if a prebuilt_file exists."""


def _impl(ctx):
    return ctx.build_setting_value

file_exists = rule(
    implementation = _impl,
    build_setting = config.bool(flag = False),
)
