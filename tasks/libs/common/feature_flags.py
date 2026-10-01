# Feature flags utilities from DDA

import json
import os
import sys

from invoke.context import Context

from tasks.libs.common.color import Color, color_message


def is_enabled(ctx: Context, feature: str, verbose: bool = True, default: bool = False) -> bool:
    """
    Performs a dda feature flag check and returns whether a feature is enabled or not.
    """
    verbose = verbose or bool(os.getenv("VERBOSE_FEATURE_FLAGS"))

    default_flag = str(default).lower()
    error = None
    try:
        res = ctx.run(f'dda self feature {feature} --default {default_flag} --json', hide=True)
        result = json.loads(res.stdout.strip())
        enabled = result['value']
        error = result.get('error')
    except Exception as e:
        error = str(e)
        enabled = default

    if verbose:
        if error:
            print(
                f'[{color_message("Warning", Color.ORANGE)}] Failed to get feature flag {feature}: {error}',
                file=sys.stderr,
            )
        if enabled or error:
            print(
                f'[{color_message("Feature", Color.BLUE)}] {color_message(feature, Color.BOLD)} is {color_message("enabled", Color.GREEN) if enabled else color_message("disabled", Color.RED)}',
                file=sys.stderr,
            )
    elif error:
        print(
            f'[{color_message("Warning", Color.ORANGE)}] Failed to get feature flag {feature}: {error}', file=sys.stderr
        )

    return enabled
