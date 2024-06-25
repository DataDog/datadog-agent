import re

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import color_message
from tasks.libs.common.git import get_current_branch


@task
def check_protected_branch(ctx):
    local_branch = get_current_branch(ctx)

    if local_branch == 'main':
        print(color_message("You're about to commit on main, are you sure this is what you want?", "red"))
        raise Exit(code=1)

    if re.fullmatch(r'^[0-9]+\.[0-9]+\.x$', local_branch):
        print(color_message("You're about to commit on a release branch, are you sure this is what you want?", "red"))
        raise Exit(code=1)
