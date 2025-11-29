# Feature flags utilities from DDA

import os
import sys

from invoke.context import Context

from tasks.libs.common.color import Color, color_message


def is_enabled(ctx: Context, feature: str, verbose: bool = False) -> bool:  # noqa
    """
    Performs a dda feature flag check and returns whether a feature is enabled or not.
    """
    verbose = verbose or bool(os.getenv("VERBOSE_FEATURE_FLAGS"))

    try:
        res = ctx.run(f'dda self feature {feature}', hide=True)
        enabled = res.stdout.strip() == 'True'
    except Exception:
        if verbose:
            print(f'[{color_message("Warning", Color.BLUE)}] Failed to get feature flag {feature}', file=sys.stderr)
        enabled = False

    if enabled or verbose:
        print(
            f'[{color_message("Feature", Color.BLUE)}] {color_message(feature, Color.BOLD)} is {color_message("enabled", Color.GREEN) if enabled else color_message("disabled", Color.RED)}',
            file=sys.stderr,
        )

    return enabled
