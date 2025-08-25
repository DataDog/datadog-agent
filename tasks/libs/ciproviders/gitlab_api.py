"""Gitlab API / CI related functions.

Provides functions to interact with the API and also helpers to manipulate and resolve gitlab-ci configurations.
"""

from __future__ import annotations

import json
import os
import platform
import re
import subprocess
import sys
from copy import deepcopy
from dataclasses import dataclass
from difflib import Differ
from functools import lru_cache
from itertools import product
from pathlib import Path
from typing import Any, Literal

import gitlab
import yaml
from gitlab.v4.objects import Project, ProjectCommit, ProjectPipeline
from invoke.exceptions import Exit

from tasks.libs.common.color import Color, color_message
from tasks.libs.common.git import get_common_ancestor, get_current_branch, get_default_branch
from tasks.libs.common.utils import retry_function
from tasks.libs.linter.gitlab_exceptions import FailureLevel, SingleGitlabLintFailure
from tasks.libs.types.types import JobDependency

BASE_URL = "https://gitlab.ddbuild.io"
CONFIG_SPECIAL_OBJECTS = {
    "default",
    "include",
    "stages",
    "variables",
    "workflow",
}


def get_gitlab_token():
    if "GITLAB_TOKEN" not in os.environ and "DD_GITLAB_TOKEN" not in os.environ:
        print("GITLAB_TOKEN / DD_GITLAB_TOKEN not found in env. Trying keychain...")
        if platform.system() == "Darwin":
            try:
                output = subprocess.check_output(
                    ['security', 'find-generic-password', '-a', os.environ["USER"], '-s', 'GITLAB_TOKEN', '-w']
                )
                if len(output) > 0:
                    return output.strip()
            except subprocess.CalledProcessError:
                print("GITLAB_TOKEN not found in keychain...")
                pass
        print(
            "Please create an 'api' access token at "
            "https://gitlab.ddbuild.io/-/profile/personal_access_tokens and "
            "add it as GITLAB_TOKEN in your keychain "
            "or export it from your .bashrc or equivalent."
        )
        raise Exit(code=1)

    return os.environ.get("GITLAB_TOKEN", os.environ.get("DD_GITLAB_TOKEN"))


def get_gitlab_bot_token():
    if "GITLAB_BOT_TOKEN" not in os.environ:
        print("GITLAB_BOT_TOKEN not found in env. Trying keychain...")
        if platform.system() == "Darwin":
            try:
                output = subprocess.check_output(
                    ['security', 'find-generic-password', '-a', os.environ["USER"], '-s', 'GITLAB_BOT_TOKEN', '-w']
                )
                if output:
                    return output.strip()
            except subprocess.CalledProcessError:
                print("GITLAB_BOT_TOKEN not found in keychain...")
                pass
        print("Please make sure that the GITLAB_BOT_TOKEN is set or that the GITLAB_BOT_TOKEN keychain entry is set.")
        raise Exit(code=1)
    return os.environ["GITLAB_BOT_TOKEN"]


def get_gitlab_api(token=None) -> gitlab.Gitlab:
    """Returns the gitlab api object with the api token.

    Args:
        The token is the one of get_gitlab_token() by default.
    """

    token = token or get_gitlab_token()

    return gitlab.Gitlab(BASE_URL, private_token=token, retry_transient_errors=True)


def get_gitlab_repo(repo='DataDog/datadog-agent', token=None) -> Project:
    api = get_gitlab_api(token)
    repo = api.projects.get(repo)

    return repo


def get_commit(project_name: str, git_sha: str) -> ProjectCommit:
    """Retrieves the commit for a given git sha a given project."""

    repo = get_gitlab_repo(project_name)
    return repo.commits.get(git_sha)


def get_pipeline(project_name: str, pipeline_id: str) -> ProjectPipeline:
    """Retrieves the pipeline for a given pipeline id in a given project."""

    repo = get_gitlab_repo(project_name)
    return repo.pipelines.get(pipeline_id)


@retry_function('refresh pipeline #{0.id}')
def refresh_pipeline(pipeline: ProjectPipeline):
    """Refreshes a pipeline, retries if there is an error."""

    pipeline.refresh()


@retry_function('cancel pipeline #{0.id}', retry_delay=5)
def cancel_pipeline(pipeline: ProjectPipeline):
    """Cancels a pipeline, retries if there is an error."""

    pipeline.cancel()


class GitlabCIDiff:
    def __init__(
        self,
        before: dict | None = None,
        after: dict | None = None,
        added: set[str] | None = None,
        removed: set[str] | None = None,
        modified: set[str] | None = None,
        renamed: set[tuple[str, str]] | None = None,
        modified_diffs: dict[str, list[str]] | None = None,
        added_contents: dict[str, str] | None = None,
    ) -> None:
        """Used to display job diffs between two gitlab ci configurations.

        Attributes:
            before: The configuration before the change.
            after: The configuration after the change.
            added: The jobs that have been added.
            removed: The jobs that have been removed.
            modified: The jobs that have been modified.
            renamed: The jobs that have been renamed.
            modified_diffs: The diffs of each of the modified jobs.
            added_contents: The contents of each of the added jobs.
        """

        self.before = before or {}
        self.after = after or {}
        self.added = added or set()
        self.removed = removed or set()
        self.modified = modified or set()
        self.renamed = renamed or set()
        self.modified_diffs = modified_diffs or {}
        self.added_contents = added_contents or {}

    def __bool__(self) -> bool:
        return bool(self.added or self.removed or self.modified or self.renamed)

    def to_dict(self) -> dict:
        return {
            'before': self.before,
            'after': self.after,
            'added': self.added,
            'removed': self.removed,
            'modified': self.modified,
            'renamed': list(self.renamed),
            'modied_diffs': self.modified_diffs,
            'added_contents': self.added_contents,
        }

    @staticmethod
    def from_dict(data: dict) -> GitlabCIDiff:
        return GitlabCIDiff(
            before=data['before'],
            after=data['after'],
            added=set(data['added']),
            removed=set(data['removed']),
            modified=set(data['modified']),
            renamed=data['renamed'],
            modified_diffs=data['modied_diffs'],
            added_contents=data['added_contents'],
        )

    @staticmethod
    def from_contents(before: dict | None = None, after: dict | None = None) -> GitlabCIDiff:
        diff = GitlabCIDiff(before, after)
        diff.make_diff()

        return diff

    def make_diff(self):
        """Computes the diff between the two gitlab ci configurations."""

        # Find added / removed jobs by names
        unmoved = self.before.keys() & self.after.keys()
        self.removed = self.before.keys() - unmoved
        self.added = self.after.keys() - unmoved
        self.renamed = set()

        # Find jobs that have been renamed
        for before_job in self.removed:
            for after_job in self.added:
                if self.before[before_job] == self.after[after_job]:
                    self.renamed.add((before_job, after_job))

        for before_job, after_job in self.renamed:
            self.removed.remove(before_job)
            self.added.remove(after_job)

        # Added jobs contents
        for job in self.added:
            self.added_contents[job] = yaml.safe_dump({job: self.after[job]}, sort_keys=True)

        # Find modified jobs
        self.modified = set()
        for job in unmoved:
            if self.before[job] != self.after[job]:
                self.modified.add(job)

        # Modified jobs
        if self.modified:
            differcli = Differ()
            for job in self.modified:
                if self.before[job] == self.after[job]:
                    continue

                # Make diff
                before_content = yaml.safe_dump({job: self.before[job]}, default_flow_style=False, sort_keys=True)
                after_content = yaml.safe_dump({job: self.after[job]}, default_flow_style=False, sort_keys=True)
                before_content = before_content.splitlines()
                after_content = after_content.splitlines()

                diff = [line.rstrip('\n') for line in differcli.compare(before_content, after_content)]
                self.modified_diffs[job] = diff

    def footnote(self, job_url: str) -> str:
        return f':information_source: *Diff available in the [job log]({job_url}).*'

    def sort_jobs(self, jobs):
        # Sort jobs by name, special objects first
        return sorted(jobs, key=lambda job: f'1{job[0]}' if job[0] in CONFIG_SPECIAL_OBJECTS else f'2{job[0]}')

    def display(
        self, cli: bool = True, max_detailed_jobs=6, job_url=None, only_summary=False, no_footnote=False
    ) -> str:
        """Returns the displayable diff in cli or markdown."""

        def str_section(title, wrap=False) -> list[str]:
            if cli:
                return [f'--- {color_message(title, Color.BOLD)} ---']
            elif wrap:
                return ['<details>', f'<summary><h3>{title}</h3></summary>']
            else:
                return [f'### {title}']

        def str_end_section(wrap: bool) -> list[str]:
            if cli:
                return []
            elif wrap:
                return ['</details>']
            else:
                return []

        def str_job(title, color):
            # Gitlab configuration special objects (variables...)
            is_special = title in CONFIG_SPECIAL_OBJECTS

            if cli:
                return f'* {color_message(title, getattr(Color, color))}{" (configuration)" if is_special else ""}'
            else:
                return f'- **{title}**{" (configuration)" if is_special else ""}'

        def str_rename(job_before, job_after):
            if cli:
                return f'* {color_message(job_before, Color.GREY)} -> {color_message(job_after, Color.BLUE)}'
            else:
                return f'- {job_before} -> **{job_after}**'

        def str_add_job(name: str, content: str) -> list[str]:
            # Gitlab configuration special objects (variables...)
            is_special = name in CONFIG_SPECIAL_OBJECTS

            if cli:
                content = [color_message(line, Color.GREY) for line in content.splitlines()]

                return [str_job(name, 'GREEN'), '', *content, '']
            else:
                header = f'<summary><b>{name}</b>{" (configuration)" if is_special else ""}</summary>'

                return ['<details>', header, '', '```yaml', *content.splitlines(), '```', '', '</details>']

        def str_modified_job(name: str, diff: list[str]) -> list[str]:
            # Gitlab configuration special objects (variables...)
            is_special = name in CONFIG_SPECIAL_OBJECTS

            if cli:
                res = [str_job(name, 'ORANGE')]
                for line in diff:
                    if line.startswith('+'):
                        res.append(color_message(line, Color.GREEN))
                    elif line.startswith('-'):
                        res.append(color_message(line, Color.RED))
                    else:
                        res.append(line)

                return res
            else:
                # Wrap diff in markdown code block and in details html tags
                return [
                    '<details>',
                    f'<summary><b>{name}</b>{" (configuration)" if is_special else ""}</summary>',
                    '',
                    '```diff',
                    *diff,
                    '```',
                    '',
                    '</details>',
                ]

        def str_color(text: str, color: str) -> str:
            if cli:
                return color_message(text, getattr(Color, color))
            else:
                return text

        def str_summary() -> str:
            if cli:
                res = ''
                res += f'{len(self.removed)} {str_color("removed", "RED")}'
                res += f' | {len(self.modified)} {str_color("modified", "ORANGE")}'
                res += f' | {len(self.added)} {str_color("added", "GREEN")}'
                res += f' | {len(self.renamed)} {str_color("renamed", "BLUE")}'

                return res
            else:
                res = '| Removed | Modified | Added | Renamed |\n'
                res += '| ------- | -------- | ----- | ------- |\n'
                res += f'| {" | ".join(str(len(changes)) for changes in [self.removed, self.modified, self.added, self.renamed])} |'

                return res

        def str_note() -> list[str]:
            if not job_url or cli:
                return []

            return ['', self.footnote(job_url)]

        res = []

        if only_summary:
            if not cli:
                res.append(':warning: Diff too large to display on Github.')
        else:
            if self.modified:
                wrap = len(self.modified) > max_detailed_jobs
                res.extend(str_section('Modified Jobs', wrap=wrap))
                for job, diff in self.sort_jobs(self.modified_diffs.items()):
                    res.extend(str_modified_job(job, diff))
                res.extend(str_end_section(wrap=wrap))

            if self.added:
                if res:
                    res.append('')
                wrap = len(self.added) > max_detailed_jobs
                res.extend(str_section('Added Jobs', wrap=wrap))
                for job, content in self.sort_jobs(self.added_contents.items()):
                    res.extend(str_add_job(job, content))
                res.extend(str_end_section(wrap=wrap))

            if self.removed:
                if res:
                    res.append('')
                res.extend(str_section('Removed Jobs'))
                for job in self.sort_jobs(self.removed):
                    res.append(str_job(job, 'RED'))

            if self.renamed:
                if res:
                    res.append('')
                res.extend(str_section('Renamed Jobs'))
                for job_before, job_after in sorted(self.renamed, key=lambda x: x[1]):
                    res.append(str_rename(job_before, job_after))

        if self.added or self.renamed or self.modified or self.removed:
            if res:
                res.append('')
            res.extend(str_section('Changes Summary'))
            res.append(str_summary())
            if not no_footnote:
                res.extend(str_note())

        return '\n'.join(res)

    def iter_jobs(self, added=False, modified=False, removed=False, only_leaves=True):
        """Will iterate over all jobs in all files for the given states.

        Args:
            only_leaves: If True, will return only leaf jobs

        Returns:
            A tuple of (job_name, contents, state)

        Notes:
            The contents of the job is the contents after modification if modified or before removal if removed
        """

        if added:
            for job in self.added:
                contents = self.after[job]
                if not only_leaves or is_leaf_job(job, contents):
                    yield job, contents, 'added'

        if modified:
            for job in self.modified:
                contents = self.after[job]
                if not only_leaves or is_leaf_job(job, contents):
                    yield job, contents, 'modified'

        if removed:
            for job in self.removed:
                contents = self.before[job]
                if not only_leaves or is_leaf_job(job, contents):
                    yield job, contents, 'removed'


class MultiGitlabCIDiff:
    @dataclass
    class MultiDiff:
        entry_point: str
        diff: GitlabCIDiff
        # Whether the entry point has been added or removed
        is_added: bool
        is_removed: bool

        def to_dict(self) -> dict:
            return {
                'entry_point': self.entry_point,
                'diff': self.diff.to_dict(),
                'is_added': self.is_added,
                'is_removed': self.is_removed,
            }

        @staticmethod
        def from_dict(data: dict) -> MultiGitlabCIDiff.MultiDiff:
            return MultiGitlabCIDiff.MultiDiff(
                data['entry_point'], GitlabCIDiff.from_dict(data['diff']), data['is_added'], data['is_removed']
            )

    def __init__(
        self,
        before: dict[str, dict] | None = None,
        after: dict[str, dict] | None = None,
        diffs: list[MultiGitlabCIDiff.MultiDiff] | None = None,
    ) -> None:
        """Used to display job diffs between two full gitlab ci configurations (multiple entry points).

        Attributes:
            before: The configuration before the change. Dict of [entry point] -> ([job name] -> job content)
            after: The configuration after the change. Dict of [entry point] -> ([job name] -> job content)
        """

        self.before = before
        self.after = after
        self.diffs = diffs or []

    def __bool__(self) -> bool:
        return bool(self.diffs)

    def to_dict(self) -> dict:
        return {'before': self.before, 'after': self.after, 'diffs': [diff.to_dict() for diff in self.diffs]}

    @staticmethod
    def from_dict(data: dict) -> MultiGitlabCIDiff:
        return MultiGitlabCIDiff(
            data['before'], data['after'], [MultiGitlabCIDiff.MultiDiff.from_dict(d) for d in data['diffs']]
        )

    @staticmethod
    def from_contents(before: dict[str, dict] | None = None, after: dict[str, dict] | None = None) -> MultiGitlabCIDiff:
        diff = MultiGitlabCIDiff(before, after)
        diff.make_diff()

        return diff

    def make_diff(self):
        self.diffs = []

        for entry_point in set(list(self.before) + list(self.after)):
            diff = GitlabCIDiff.from_contents(self.before.get(entry_point, {}), self.after.get(entry_point, {}))

            # Diff for this entry point, add it to the list
            if diff:
                self.diffs.append(
                    MultiGitlabCIDiff.MultiDiff(
                        entry_point, diff, entry_point not in self.before, entry_point not in self.after
                    )
                )

    def display(self, cli: bool = True, job_url: str = None, **kwargs) -> str:
        """Returns the displayable diff in cli or markdown."""

        if not self:
            return ''

        if len(self.diffs) == 1:
            return self.diffs[0].diff.display(cli, job_url=job_url, **kwargs)

        def str_entry(diff: MultiGitlabCIDiff.MultiDiff) -> str:
            if cli:
                status = ''
                if diff.is_added:
                    status = f'{color_message("Added:", Color.GREEN)} '
                elif diff.is_removed:
                    status = f'{color_message("Removed:", Color.RED)} '
                else:
                    status = f'{color_message("Modified:", Color.ORANGE)} '

                return [f'>>> {status}{color_message(diff.entry_point, Color.BOLD)}', '']
            else:
                status = ''
                if diff.is_added:
                    status = 'Added: '
                elif diff.is_removed:
                    status = 'Removed: '
                else:
                    status = 'Updated: '

                return [f'<details><summary><h2>{status}{diff.entry_point}</h2></summary>', '']

        def str_entry_end() -> list[str]:
            if cli:
                return ['']
            else:
                return ['', '</details>']

        res = []

        # .gitlab-ci.yml will be always first and other entries sorted alphabetically
        diffs = sorted(self.diffs, key=lambda diff: '' if diff.entry_point == '.gitlab-ci.yml' else diff.entry_point)

        for diff in diffs:
            res.extend(str_entry(diff))
            res.append(diff.diff.display(cli, job_url=job_url, no_footnote=True, **kwargs))
            res.extend(str_entry_end())

        if not cli:
            res.append('')
            res.append(self.diffs[-1].diff.footnote(job_url))

        return '\n'.join(res)

    def iter_jobs(self, added=False, modified=False, removed=False, only_leaves=True):
        """Will iterate over all jobs in all files for the given states.

        Args:
            only_leaves: If True, will return only leaf jobs.

        Returns:
            A tuple of (entry_point, job_name, contents, state).

        Notes:
            The contents is the contents after modification or before removal.
        """

        for diff in self.diffs:
            for job, contents, state in diff.diff.iter_jobs(
                added=added, modified=modified, removed=removed, only_leaves=only_leaves
            ):
                yield diff.entry_point, job, contents, state


class ReferenceTag(yaml.YAMLObject):
    """Custom yaml tag to handle references (!reference [...]) in gitlab-ci configuration."""

    yaml_tag = '!reference'

    def __init__(self, references):
        self.references = references

    def __iter__(self):
        return iter(self.references)

    def __str__(self):
        return f'{self.yaml_tag} {self.references}'

    @classmethod
    def from_yaml(cls, loader, node):
        return ReferenceTag(loader.construct_sequence(node))

    @classmethod
    def to_yaml(cls, dumper, data: ReferenceTag):
        return dumper.represent_sequence(cls.yaml_tag, data.references, flow_style=True)


# Update loader/dumper to handle !reference tag
yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
yaml.SafeDumper.add_representer(ReferenceTag, ReferenceTag.to_yaml)

# HACK: The following line is a workaround to prevent yaml dumper from removing quote around comma separated numbers, otherwise Gitlab Lint API will remove the commas
yaml.SafeDumper.add_implicit_resolver(
    'tag:yaml.org,2002:int', re.compile(r'''^([0-9]+(,[0-9]*)*)$'''), list('0213456789')
)


def clean_gitlab_ci_configuration(yml: dict) -> dict:
    """Cleans up a gitlab-ci configuration object by:
    - Removing `extends` tags.
    - Flattening lists of lists.
    """

    def flatten(yml):
        """
        Flatten lists (nesting due to !reference tags)
        """
        if isinstance(yml, list):
            res = []
            for v in yml:
                if isinstance(v, list):
                    res.extend(flatten(v))
                else:
                    res.append(v)

            return res
        elif isinstance(yml, dict):
            return {k: flatten(v) for k, v in yml.items()}
        else:
            return yml

    # Remove extends
    for content in yml.values():
        if 'extends' in content:
            del content['extends']

    # Flatten
    return flatten(yml)  # type: ignore


def is_leaf_job(job_name, job_contents):
    """A 'leaf' job is a job that will be executed by gitlab-ci, that is a job that is not meant to be only extended (usually jobs starting with '.') or special gitlab objects (variables, stages...)."""

    return not job_name.startswith('.') and ('script' in job_contents or 'trigger' in job_contents)


def filter_gitlab_ci_configuration(
    yml: dict, jobs: str | set[str] | None = None, keep_special_objects: bool = False
) -> dict:
    """Filters elements in a gitlab-ci configuration object

    Args:
        yml: The gitlab-ci configuration object to filter
        jobs:
            If provided, retrieve only these jobs.
            Note that an empty set means no jobs will be returned !
            For convenience, you can also provide a single str.
        keep_special_objects: Will keep special objects (not jobs) in the configuration (variables, stages, etc.).
    """

    if isinstance(jobs, str):
        jobs = {jobs}

    def filter_yaml(key, value):
        # Not a job
        if not is_leaf_job(key, value):
            # Exception for special objects if this option is enabled
            if not (keep_special_objects and key in CONFIG_SPECIAL_OBJECTS):
                return None

        if jobs is not None:
            return (key, value) if key in jobs else None

        return key, value

    if jobs is not None:
        for job in jobs:
            assert job in yml, f"Job {job} not found in the configuration"

    return {node[0]: node[1] for node in (filter_yaml(k, v) for k, v in yml.items()) if node is not None}


def _get_combinated_variables(arg: dict[str, (str | list[str])]):
    """Make combinations from the matrix arguments to obtain the list of variables that have each new job.

    combinations({'key1': ['val1', 'val2'], 'key2': 'val3'}) -> [
        {'key1': 'val1', 'key2': 'val3'},
        {'key1': 'val2', 'key2': 'val3'}
    ]

    Returns:
        A tuple of (1) the list of variable values and (2) the list of variable dictionaries
    """

    job_keys = []
    job_values = []
    for key, values in arg.items():
        if not isinstance(values, list):
            values = [values]

        job_keys.append([key] * len(values))
        job_values.append(values)

    # Product order is deterministic so each item in job_values will be associated with the same item in job_keys
    job_keys = list(product(*job_keys))
    job_values = list(product(*job_values))

    job_vars = [dict(zip(k, v, strict=True)) for (k, v) in zip(job_keys, job_values, strict=True)]

    return job_values, job_vars


def expand_matrix_jobs(yml: dict) -> dict:
    """Will expand matrix jobs into multiple jobs."""

    new_jobs = {}
    to_remove = set()
    for job in yml:
        if 'parallel' in yml[job] and 'matrix' in yml[job]['parallel']:
            to_remove.add(job)
            for arg in yml[job]['parallel']['matrix']:
                # Compute all combinations of variables
                job_values, job_vars = _get_combinated_variables(arg)

                # Create names
                job_names = [', '.join(str(value) for value in spec) for spec in job_values]
                job_names = [f'{job}: [{specs}]' for specs in job_names]

                for variables, name in zip(job_vars, job_names, strict=True):
                    new_job = deepcopy(yml[job])

                    # Update variables
                    new_job['variables'] = {**new_job.get('variables', {}), **variables}

                    # Remove matrix config for the new jobs
                    del new_job['parallel']['matrix']
                    if not new_job['parallel']:
                        del new_job['parallel']

                    new_jobs[name] = new_job

    for job in to_remove:
        del yml[job]

    yml.update(new_jobs)

    return yml


def print_gitlab_ci_configuration(yml: dict, sort_jobs: bool):
    """Prints a gitlab ci as yaml.

    Args:
        sort_jobs: Sort jobs by name (the job keys are always sorted).
    """

    jobs = yml.items()
    if sort_jobs:
        jobs = sorted(jobs)

    for i, (job, content) in enumerate(jobs):
        if i > 0:
            print()
        yaml.safe_dump({job: content}, sys.stdout, default_flow_style=False, sort_keys=True, indent=2)


def test_gitlab_configuration(entry_point: str, config_object: dict, context=None):
    agent = get_gitlab_repo()
    # Apply the new variables from context
    # Important to DISABLE CLEAN AND FILTERING - the config at this point is minimally resolved (only includes)
    # Thus, removing anything like `extends` and dotted jobs will render the config invalid
    config_object = post_process_gitlab_ci_configuration(
        config_object, variable_overrides=context, do_filtering=False, clean=False
    )
    config_dump = yaml.safe_dump(config_object)
    res = agent.ci_lint.create({"content": config_dump, "dry_run": True, "include_jobs": True})
    if len(res.warnings) > 0:
        raise SingleGitlabLintFailure(
            entry_point=entry_point,
            _level=FailureLevel.WARNING,
            _details=f"Gitlab CI configuration has warnings: {res.warnings}",
        )
    if not res.valid:
        raise SingleGitlabLintFailure(
            entry_point=entry_point,
            _level=FailureLevel.ERROR,
            _details=f"Gitlab CI configuration is invalid: {res.errors}",
        )


def post_process_gitlab_ci_configuration(
    config: dict,
    clean: bool = True,
    variable_overrides: dict | None = None,
    do_filtering: bool = False,
    filter_jobs: str | set[str] | None = None,
    keep_special_objects: bool = False,
    expand_matrix: bool = False,
    **kwargs,
) -> dict[str, Any]:
    """Apply post-processing functions to a gitlabci config object.
    See argument reference for a list of options.

    Args:
        config: The gitlab config object to apply post-processing to.
        variable_overrides: Dictionary containing variables to override in the output config (gitlab `variables` tag).
        do_filtering: Whether to apply the `filter_gitlab_ci_configuration` method to the provided config. If `filter_jobs` or `keep_special_objects` are set, this will be considered True.
        filter_jobs:
            If not None, only the job objects with names specified here will be included in the config.
            Note that an empty set means no jobs will be returned !
            For convenience, you can also provide a single str.
        keep_special_objects: Will keep special objects (not jobs) in the configuration (variables, stages, etc.).
        expand_matrix: Will expand matrix jobs into multiple jobs.
    """
    # Make sure to deepcopy the input config object before modifying it
    # Otherwise this can have weird side effects when testing different config objects
    # Ex: multiple contexts in the `gitlab-ci` task
    processed_config = deepcopy(config)

    # Apply filtering
    if do_filtering or filter_jobs or keep_special_objects:
        processed_config = filter_gitlab_ci_configuration(processed_config, filter_jobs, keep_special_objects)

    if clean:
        processed_config = clean_gitlab_ci_configuration(processed_config)

    # Expand matrix jobs
    if expand_matrix:
        processed_config = expand_matrix_jobs(processed_config)

    # Override some variables with a dedicated context
    if variable_overrides:
        processed_config.get('variables', {}).update(variable_overrides)

    return processed_config


def get_all_gitlab_ci_configurations(
    ctx,
    input_file: str = '.gitlab-ci.yml',
    resolve_only_includes: bool = False,
    git_ref: str | None = None,
    postprocess_options: dict[str, Any] | Literal[False] | None = None,
) -> dict[str, dict]:
    """Returns all possible gitlab CI entrypoints and corresponding fully-resolved configurations, rooted at the input file.
    This is useful when the CI contains 'trigger jobs', which launch new, independent pipelines.
    These 'triggered pipelines' are syntactically independent and as such must be handled independently.

    Args:
        input_file: Path to a gitlab CI configuration file from which to constructing.
        ignore_errors: Ignore gitlab lint errors.
        resolve_only_includes:
            Whether to skip the gitlab `/lint` endpoint when resolving configs.
            In this case, only `include`s will be resolved, not `extend`s or `!reference`s
        postprocess_options:
            Controls how postprocessing is done.
            You can pass a dict of options that will be passed to `post_process_gitlab_ci_configuration` for each resolved config object, overriding that function's parameters.
            When None (default), this has the same effect as passing in an empty dict -- all the default arguments of `post_process_gitlab_ci_configuration` will be used.
            If `false`, post-processing will be DISABLED.
    Returns:
        A dictionary of [entry point] -> configuration
    """
    print(f'[{color_message("INFO", Color.BLUE)}] Fetching Gitlab CI configurations...')

    # configurations[input_file] -> parsed config
    configurations: dict[str, dict] = {}

    # Traverse all gitlab-ci configurations
    _traverse_config_search_triggers(
        input_file, configurations=configurations, ctx=ctx, resolve_only_includes=resolve_only_includes, git_ref=git_ref
    )
    # Post process
    # Note: the is check with False is not an error - we want to skip postprocessing when it is exactly `False`, not just a Falsy value.
    if postprocess_options is False:
        return configurations

    if postprocess_options is None:
        postprocess_options = {}

    if postprocess_options is not False:
        for file_name, config in configurations.items():
            configurations[file_name] = post_process_gitlab_ci_configuration(config, **postprocess_options)  # type: ignore

    return configurations


def _traverse_config_search_triggers(
    input_file: str, configurations: dict[str, dict], ctx, resolve_only_includes, git_ref
) -> None:
    """
    Produces a DFS to discover all possible gitlab CI entrypoints and corresponding configurations, rooted at the input file.
    THIS IS A RECURSIVE HELPER FUNCTION: Use `get_all_gitlab_ci_configurations` instead.
    This is useful when the CI contains 'trigger jobs', which launch new, independent pipelines.
    These 'triggered pipelines' are syntactically independent and as such must be handled independently.

    Args:
        input_file: Path to a gitlab CI configuration file from which to start constructing the DFS.
        configurations: Dictionary object storing the result. THIS ARGUMENT WILL BE WRITTEN TO.
        ctx: Invoke task context
        resolve_only_includes:
            Whether to skip the gitlab `/lint` endpoint when resolving configs.
            In this case, only `include`s will be resolved, not `extend`s or `!reference`s
        git_ref: What git ref to use when opening the `input_file`
    """

    if input_file in configurations:
        return

    # Read and parse the configuration from this input_file
    config = resolve_gitlab_ci_configuration(
        ctx, input_file, resolve_only_includes=resolve_only_includes, git_ref=git_ref
    )
    configurations[input_file] = config

    # Search and add configurations called by the trigger keyword
    for job in config.values():
        if 'trigger' in job and 'include' in job['trigger']:
            for trigger in _get_trigger_filenames(job['trigger']['include']):
                _traverse_config_search_triggers(
                    trigger,
                    configurations=configurations,
                    ctx=ctx,
                    resolve_only_includes=resolve_only_includes,
                    git_ref=git_ref,
                )


def _get_trigger_filenames(node):
    """Gets all trigger downstream pipelines defined by the `trigger` key in the gitlab-ci configuration."""

    if isinstance(node, str):
        return [node]
    elif isinstance(node, dict):
        return [node['local']] if 'local' in node else []
    elif isinstance(node, list):
        res = []
        for n in node:
            res.extend(_get_trigger_filenames(n))

        return res


def resolve_gitlab_ci_configuration(
    ctx,
    input_config_or_file: str | dict = '.gitlab-ci.yml',
    resolve_only_includes: bool = False,
    git_ref: str | None = None,
) -> dict:
    """Returns a full gitlab-ci configuration object by resolving all `include`s, `extends`s and `!reference`s.

    If resolve_only_includes is True, only `include`s will be resolved.
    Otherwise, the `/lint` Gitlab API endpoint will be called, fully resolving the config.

    Args:
        ctx: Invoke task context
        input_config_or_file: The gitlab config to resolve, either as a path to a file or a loaded dict
        return_dict: Return a loaded dict - If false, return a yaml string representing the config
        resolve_only_includes:
            Whether to skip the gitlab `/lint` endpoint when resolving configs.
            In this case, only `include`s will be resolved, not `extend`s or `!reference`s
        git_ref: From which git ref to read the input config file. No effect if input config is passed as a dict.
    """

    # Read includes
    input_config = read_includes(ctx, input_config_or_file, return_config=True, git_ref=git_ref)
    assert input_config

    if resolve_only_includes:
        return input_config

    agent = get_gitlab_repo()
    res = agent.ci_lint.create({"content": yaml.safe_dump(input_config), "dry_run": True, "include_jobs": True})

    if not res.valid:
        errors = '; '.join(res.errors)
        raise RuntimeError(f"{color_message('Invalid configuration', Color.RED)}: {errors}")

    return yaml.safe_load(res.merged_yaml)


def read_includes(ctx, yaml_files, includes=None, return_config=False, add_file_path=False, git_ref: str | None = None):
    """Recursive method to read all includes from yaml files and store them in a list.

    Args:
        add_file_path: add the file path to each object of the parsed file.
    """

    if includes is None:
        includes = []

    if isinstance(yaml_files, str):
        yaml_files = [yaml_files]

    for yaml_file in yaml_files:
        current_file = read_content(ctx, yaml_file, git_ref=git_ref)

        if add_file_path:
            for value in current_file.values():
                if isinstance(value, dict):
                    value['_file_path'] = yaml_file

        if 'include' not in current_file:
            includes.append(current_file)
        else:
            read_includes(ctx, current_file['include'], includes, add_file_path=add_file_path, git_ref=git_ref)
            del current_file['include']
            includes.append(current_file)

    # Merge all files
    if return_config:
        full_configuration = {}
        for yaml_file in includes:
            full_configuration.update(yaml_file)

        return full_configuration


def read_content(ctx, file_path, git_ref: str | None = None):
    """Reads the content of a file, either from a local file or from an http endpoint."""

    # Do not use ctx for cache
    @lru_cache(maxsize=512)
    def read_content_cached(file_path, git_ref: str | None = None):
        nonlocal ctx

        if file_path.startswith('http'):
            import requests

            response = requests.get(file_path)
            response.raise_for_status()
            content = response.text
        elif git_ref:
            content = ctx.run(f"git show '{git_ref}:{file_path}'", hide=True).stdout
        else:
            with open(file_path) as f:
                content = f.read()

        return yaml.safe_load(content)

    return read_content_cached(file_path, git_ref)


def get_preset_contexts(required_tests):
    possible_tests = ["all", "main", "release", "mq", "conductor"]
    required_tests = required_tests.casefold().split(",")
    if set(required_tests) | set(possible_tests) != set(possible_tests):
        raise Exit(f"Invalid test required: {required_tests} must contain only values from {possible_tests}", 1)
    main_contexts = [
        ("BUCKET_BRANCH", ["nightly"]),  # ["dev", "nightly", "beta", "stable", "oldnightly"]
        ("CI_COMMIT_BRANCH", ["main"]),  # ["main", "mq-working-branch-main", "7.42.x", "any/name"]
        ("CI_PIPELINE_SOURCE", ["push", "api"]),  # ["trigger", "pipeline", "schedule"]
        ("DEPLOY_AGENT", ["true"]),
        ("RUN_ALL_BUILDS", ["true"]),
        ("RUN_E2E_TESTS", ["auto"]),
        ("RUN_KMT_TESTS", ["on"]),
        ("RUN_UNIT_TESTS", ["on"]),
    ]
    release_contexts = [
        ("BUCKET_BRANCH", ["stable"]),
        ("CI_COMMIT_BRANCH", ["7.42.x"]),
        ("CI_COMMIT_TAG", ["3.2.1", "1.2.3-rc.4"]),
        ("CI_PIPELINE_SOURCE", ["schedule"]),
        ("DEPLOY_AGENT", ["true"]),
        ("RUN_ALL_BUILDS", ["true"]),
        ("RUN_E2E_TESTS", ["auto"]),
        ("RUN_KMT_TESTS", ["on"]),
        ("RUN_UNIT_TESTS", ["on"]),
    ]
    mq_contexts = [
        ("BUCKET_BRANCH", ["dev"]),
        ("CI_COMMIT_BRANCH", ["mq-working-branch-main"]),
        ("CI_PIPELINE_SOURCE", ["api"]),
        ("DEPLOY_AGENT", ["false"]),
        ("RUN_ALL_BUILDS", ["false"]),
        ("RUN_E2E_TESTS", ["auto"]),
        ("RUN_KMT_TESTS", ["off"]),
        ("RUN_UNIT_TESTS", ["off"]),
    ]
    conductor_contexts = [
        ("BUCKET_BRANCH", ["nightly"]),  # ["dev", "nightly", "beta", "stable", "oldnightly"]
        ("CI_COMMIT_BRANCH", ["main"]),  # ["main", "mq-working-branch-main", "7.42.x", "any/name"]
        ("CI_PIPELINE_SOURCE", ["pipeline"]),  # ["trigger", "pipeline", "schedule"]
        ("DDR_WORKFLOW_ID", ["true"]),
    ]
    installer_contexts = [
        ("BUCKET_BRANCH", ["nightly"]),
        ("CI_COMMIT_BRANCH", ["main"]),
        ("CI_PIPELINE_SOURCE", ["push", "api"]),
        ("DEPLOY_AGENT", ["false"]),
        ("DEPLOY_INSTALLER", ["true"]),
        ("RUN_ALL_BUILDS", ["true"]),
        ("RUN_E2E_TESTS", ["auto"]),
        ("RUN_KMT_TESTS", ["on"]),
        ("RUN_UNIT_TESTS", ["on"]),
    ]
    integrations_core_contexts = [
        ("BUCKET_BRANCH", ["dev"]),
        ("DEPLOY_AGENT", ["false"]),
        ("CI_PIPELINE_SOURCE", ["pipeline"]),  # ["trigger", "pipeline", "schedule"]
        ("INTEGRATIONS_CORE_VERSION", ["foo/bar"]),
        ("RUN_KITCHEN_TESTS", ["false"]),
        ("RUN_E2E_TESTS", ["off"]),
    ]
    all_contexts = []
    for test in required_tests:
        if test in ["all", "main"]:
            generate_contexts(main_contexts, [], all_contexts)
        if test in ["all", "release"]:
            generate_contexts(release_contexts, [], all_contexts)
        if test in ["all", "mq"]:
            generate_contexts(mq_contexts, [], all_contexts)
        if test in ["all", "conductor"]:
            generate_contexts(conductor_contexts, [], all_contexts)
        if test in ["all", "installer"]:
            generate_contexts(installer_contexts, [], all_contexts)
        if test in ["all", "integrations"]:
            generate_contexts(integrations_core_contexts, [], all_contexts)
    return all_contexts


def generate_contexts(contexts, context, all_contexts):
    """
    Recursive method to generate all possible contexts from a list of tuples
    """
    if len(contexts) == 0:
        all_contexts.append(context[:])
        return
    for value in contexts[0][1]:
        context.append((contexts[0][0], value))
        generate_contexts(contexts[1:], context, all_contexts)
        context.pop()


def load_context(context):
    """Loads a context either from a yaml file or from a json string."""

    if os.path.exists(context):
        with open(context) as f:
            y = yaml.safe_load(f)
        if "variables" not in y:
            raise Exit(
                f"Invalid context file: {context}, missing 'variables' key. Input file must be similar to tasks/unit-tests/testdata/gitlab_main_context_template.yml",
                1,
            )
        return [list(y["variables"].items())]
    else:
        try:
            j = json.loads(context)
            return [list(j.items())]
        except json.JSONDecodeError as e:
            raise Exit(f"Invalid context: {context}, must be a valid json, or a path to a yaml file", 1) from e


def retrieve_all_paths(yaml):
    if isinstance(yaml, dict):
        for key, value in yaml.items():
            if key == "changes":
                if isinstance(value, list):
                    yield from value
                elif "paths" in value:
                    yield from value["paths"]
            yield from retrieve_all_paths(value)
    elif isinstance(yaml, list):
        for item in yaml:
            yield from retrieve_all_paths(item)


def gitlab_configuration_is_modified(ctx):
    branch = get_current_branch(ctx)
    if branch == "main":
        # We usually squash merge on main so comparing only to the last commit
        diff = ctx.run("git diff HEAD^1..HEAD", hide=True).stdout.strip().splitlines()
    else:
        # On dev branch we compare all the new commits
        ctx.run("git fetch origin main:main")
        ancestor = get_common_ancestor(ctx, branch)
        diff = ctx.run(f"git diff {ancestor}..HEAD", hide=True).stdout.strip().splitlines()
    modified_files = re.compile(r"^diff --git a/(.*) b/(.*)")
    changed_lines = re.compile(r"^@@ -\d+,\d+ \+(\d+),(\d+) @@")
    leading_space = re.compile(r"^(\s*).*$")
    in_config = False
    for line in diff:
        if line.startswith("diff --git"):
            files = modified_files.match(line)
            new_file = files.group(1)
            # Third condition is only for testing purposes
            if (
                new_file.startswith(".gitlab") and new_file.endswith(".yml")
            ) or "testdata/yaml_configurations" in new_file:
                in_config = True
                print(f"Found a gitlab configuration file: {new_file}")
            else:
                in_config = False
        if in_config and line.startswith("@@") and os.path.exists(new_file):
            lines = changed_lines.match(line)
            start = int(lines.group(1))
            with open(new_file) as f:
                content = f.readlines()
                item = leading_space.match(content[start])
                if item:
                    for above_line in reversed(content[:start]):
                        current = leading_space.match(above_line)
                        if current[1] < item[1]:
                            if any(keyword in above_line for keyword in ["needs:", "dependencies:"]):
                                print(f"> Found a gitlab configuration change on line: {content[start]}")
                                return True
                            else:
                                break
        if (
            in_config
            and line.startswith("+")
            and (
                (len(line) > 1 and line[1].isalpha())
                or any(keyword in line for keyword in ["needs:", "dependencies:", "!reference"])
            )
        ):
            print(f"> Found a gitlab configuration change on line: {line}")
            return True

    return False


def compute_gitlab_ci_config_diff(ctx, before: str | None = None, after: str | None = None):
    """Computes the full configs and the diff between two git references.

    The "after reference" is compared to the Lowest Common Ancestor (LCA) commit of "before reference" and "after reference".

    Args:
        before: The git reference to compare to (default: default branch).
        after: The git reference to compare (default: local files).
    """

    before_name = before or "merge base"
    after_name = after or "local files"

    # The before commit is the LCA commit between before and after
    before = before or get_default_branch()
    before = get_common_ancestor(ctx, before, after or "HEAD")

    print(f'Getting after changes config ({color_message(after_name, Color.BOLD)})')
    after_config = get_all_gitlab_ci_configurations(ctx, git_ref=after)

    print(f'Getting before changes config ({color_message(before_name, Color.BOLD)})')
    before_config = get_all_gitlab_ci_configurations(ctx, git_ref=before)

    diff = MultiGitlabCIDiff.from_contents(before_config, after_config)

    return before_config, after_config, diff


def full_config_get_all_leaf_jobs(full_config: dict) -> set[str]:
    """Filters all leaf jobs from a full gitlab-ci configuration.

    Returns:
        A set containing all leaf jobs. A leaf job is a job that will be executed by gitlab-ci, that is a job that is not meant to be only extended (usually jobs starting with '.') or special gitlab objects (variables, stages...).
    """

    all_jobs = set()
    for config in full_config.values():
        all_jobs.update({job for job in config if is_leaf_job(job, config[job])})

    return all_jobs


def full_config_get_all_stages(full_config: dict) -> set[str]:
    """Retrieves all stages from a full gitlab-ci configuration."""

    all_stages = set()
    for config in full_config.values():
        all_stages.update(config.get("stages", []))

    return all_stages


def update_test_infra_def(file_path, image_tag, is_dev_image=False, prefix_comment=""):
    """
    Updates TEST_INFRA_DEFINITIONS_BUILDIMAGES in `.gitlab/common/test_infra_version.yml` file
    """
    test_infra_def = {}
    with open(file_path) as test_infra_version_file:
        try:
            test_infra_def = yaml.safe_load(test_infra_version_file)
            test_infra_def["variables"]["TEST_INFRA_DEFINITIONS_BUILDIMAGES"] = image_tag
            if is_dev_image:
                test_infra_def["variables"]["TEST_INFRA_DEFINITIONS_BUILDIMAGES_SUFFIX"] = "-dev"
            else:
                test_infra_def["variables"]["TEST_INFRA_DEFINITIONS_BUILDIMAGES_SUFFIX"] = ""
        except yaml.YAMLError as e:
            raise Exit(f"Error while loading {file_path}: {e}") from e
    with open(file_path, "w") as test_infra_version_file:
        test_infra_version_file.write(prefix_comment + ('\n\n' if prefix_comment else ''))
        # Add explicit_start=True to keep the document start marker ---
        # See "Document Start" in https://www.yaml.info/learn/document.html for more details
        yaml.dump(test_infra_def, test_infra_version_file, explicit_start=True)


def get_test_infra_def_version():
    """
    Get TEST_INFRA_DEFINITIONS_BUILDIMAGES from `.gitlab/common/test_infra_version.yml` file
    """
    try:
        version_file = Path.cwd() / ".gitlab" / "common" / "test_infra_version.yml"
        test_infra_def = yaml.safe_load(version_file.read_text(encoding="utf-8"))
        return test_infra_def["variables"]["TEST_INFRA_DEFINITIONS_BUILDIMAGES"]
    except Exception:
        return "main"


def get_buildimages_version():
    """
    Get the version of datadog-agent-buildimages currently used
    """
    try:
        version_file = Path.cwd() / ".gitlab-ci.yml"
        gitlab_ci_yaml = yaml.safe_load(version_file.read_text(encoding="utf-8"))
        # CI_IMAGE_DEB_ARM64 is an approximation we agreed on with DevX folks
        return gitlab_ci_yaml["variables"]["CI_IMAGE_DEB_ARM64"].split("-")[1].strip()
    except Exception:
        return "main"


def update_gitlab_config(file_path, tag, images="", test=True, update=True, windows=False):
    """
    Override variables in .gitlab-ci.yml file.
    """
    with open(file_path) as gl:
        file_content = gl.readlines()
    yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
    gitlab_ci = yaml.safe_load("".join(file_content))
    variables = gitlab_ci['variables']
    # Select the buildimages prefixed with CI_IMAGE matchins input images list
    images_to_update = list(find_buildimages(variables, images, windows=windows))
    if update:
        output = update_image_tag(file_content, tag, images_to_update, test=test)
        with open(file_path, "w") as gl:
            gl.writelines(output)
    return images_to_update


def find_buildimages(variables, images="", prefix="CI_IMAGE_", windows=False):
    """
    Select the buildimages variables to update.
    With default values, the former CI_IMAGE_ variables are updated.
    """
    suffix = "_SUFFIX"
    for variable in variables:
        if (
            variable.startswith(prefix)
            and variable.endswith(suffix)
            and any(image in variable.casefold() for image in images.casefold().split(","))
        ):
            if 'WIN' in variable and not windows:
                continue
            yield variable.removesuffix(suffix)


def update_image_tag(lines, tag, variables, test=True):
    """
    Update the variables in the .gitlab-ci.yml file.
    We update the file content (instead of the yaml.load) to keep the original order/formatting.
    """
    output = []
    tag_pattern = re.compile(r"v\d+-\w+")
    for line in lines:
        if any(variable in line for variable in variables):
            if "SUFFIX" in line:
                if test:
                    output.append(line.replace('""', '"_test_only"'))
                else:
                    output.append(line.replace('"_test_only"', '""'))
            else:
                is_tag = tag_pattern.search(line)
                if is_tag:
                    output.append(line.replace(is_tag.group(), tag))
                else:
                    output.append(line)
        else:
            output.append(line)
    return output


def get_gitlab_job_dependencies(gitlab_cfg: dict, job_name: str) -> list[JobDependency]:
    """
    Get the dependencies of a job from the gitlab configuration.
    """
    job_dependencies = []
    job_data = gitlab_cfg[job_name]
    if "needs" in job_data:
        for need in job_data["needs"]:
            job_name = need["job"]
            matrix_needs = need.get('parallel', {}).get('matrix', [])
            tags = []
            for matrix_need in matrix_needs:
                need_tags = []
                for tag_name, tag_values in matrix_need.items():
                    if isinstance(tag_values, str):
                        tag_values = [tag_values]
                    need_tags.append((tag_name, set(tag_values)))
                tags.append(need_tags)

            job_dependencies.append(JobDependency(job_name, tags))
    return job_dependencies
