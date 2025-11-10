"""
Diffing tasks
"""

import datetime
import json
from datetime import timedelta

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.worktree import agent_context


@task
def go_deps(ctx):
    raise Exit("This task has been renamed `go-deps.diff`")


def _list_tasks_rec(collection, prefix='', res=None):
    res = res or {}

    if isinstance(collection, dict):
        newpref = prefix + collection['name']

        for task in collection['tasks']:
            res[newpref + '.' + task['name']] = task['help']

        for subtask in collection['collections']:
            _list_tasks_rec(subtask, newpref + '.', res)

    return res


def _list_invoke_tasks(ctx) -> dict[str, str]:
    """Returns a dictionary of invoke tasks and their descriptions."""

    tasks = json.loads(ctx.run('dda inv -- --list -F json', hide=True).stdout)

    # Remove 'tasks.' prefix
    return {name.removeprefix(tasks['name'] + '.'): desc for name, desc in _list_tasks_rec(tasks).items()}


@task
def invoke_tasks(ctx, diff_date: str | None = None):
    """Shows the added / removed invoke tasks since diff_date with their description.

    Args:
        diff_date: The date to compare the tasks to ('YYYY-MM-DD' format). Will be the last 30 days if not provided.
    """

    if not diff_date:
        diff_date = (datetime.datetime.now() - timedelta(days=30)).strftime('%Y-%m-%d')
    else:
        try:
            datetime.datetime.strptime(diff_date, '%Y-%m-%d')
        except ValueError as e:
            raise Exit('Invalid date format. Please use the format "YYYY-MM-DD".') from e

    old_commit = ctx.run(f"git rev-list -n 1 --before='{diff_date} 23:59' HEAD", hide=True).stdout.strip()
    assert old_commit, f"No commit found before {diff_date}"

    with agent_context(ctx, commit=old_commit):
        old_tasks = _list_invoke_tasks(ctx)
    current_tasks = _list_invoke_tasks(ctx)

    all_tasks = set(old_tasks.keys()).union(current_tasks.keys())
    removed_tasks = {task for task in all_tasks if task not in current_tasks}
    added_tasks = {task for task in all_tasks if task not in old_tasks}

    if removed_tasks:
        print(f'* {color_message("Removed tasks", Color.BOLD)}:')
        print('\n'.join(sorted(f'- {name}' for name in removed_tasks)))
    else:
        print('No task removed')

    if added_tasks:
        print(f'\n* {color_message("Added tasks", Color.BOLD)}:')
        for name, description in sorted((name, current_tasks[name]) for name in added_tasks):
            line = '+ ' + name
            if description:
                line += ': ' + description
            print(line)
    else:
        print('No task added')
