"""Other/miscellaneous linting-related tasks"""

import sys

from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.common.git import get_staged_files
from tasks.libs.types.copyright import CopyrightLinter, LintFailure


@task
def copyrights(ctx, fix=False, dry_run=False, debug=False, only_staged_files=False):
    """Checks that all Go files contain the appropriate copyright header.

    If '--fix' is provided as an option, it will try to fix problems as it finds them.
    If '--dry_run' is provided when fixing, no changes to the files will be applied.
    """

    files = None

    if only_staged_files:
        staged_files = get_staged_files(ctx)
        files = [path for path in staged_files if path.endswith(".go")]

    try:
        CopyrightLinter(debug=debug).assert_compliance(fix=fix, dry_run=dry_run, files=files)
    except LintFailure:
        # the linter prints useful messages on its own, so no need to print the exception
        sys.exit(1)


@task
def filenames(ctx):
    """Scans files to ensure there are no filenames too long or containing illegal characters."""

    files = ctx.run("git ls-files -z", hide=True).stdout.split("\0")
    failure = False

    if sys.platform == 'win32':
        print("Running on windows, no need to check filenames for illegal characters")
    else:
        print("Checking filenames for illegal characters")
        forbidden_chars = '<>:"\\|?*'
        for filename in files:
            if any(char in filename for char in forbidden_chars):
                print(f"Error: Found illegal character in path {filename}")
                failure = True

    print("Checking filename length")
    # Approximated length of the prefix of the repo during the windows release build
    prefix_length = 160
    # Maximum length supported by the win32 API
    max_length = 255
    for filename in files:
        if (
            not filename.startswith(('tools/windows/DatadogAgentInstaller', 'test/workload-checks', 'test/regression'))
            and prefix_length + len(filename) > max_length
        ):
            print(
                f"Error: path {filename} is too long ({prefix_length + len(filename) - max_length} characters too many)"
            )
            failure = True

    if failure:
        raise Exit(code=1)
