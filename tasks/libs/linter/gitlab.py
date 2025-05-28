from __future__ import annotations

import re

import yaml
from invoke.exceptions import Exit

from tasks.libs.ciproviders.gitlab_api import (
    MultiGitlabCIDiff,
    is_leaf_job,
)
from tasks.libs.common.color import Color, color_message


class SSMParameterCall:
    def __init__(self, file, line_nb, with_wrapper=False, with_env_var=False):
        """
        Initialize an SSMParameterCall instance.

        Args:
            file (str): The name of the file where the SSM parameter call is located.
            line_nb (int): The line number in the file where the SSM parameter call is located.
            with_wrapper (bool, optional): If the call is using the wrapper. Defaults to False.
            with_env_var (bool, optional): If the call is using an environment variable defined in .gitlab-ci.yml. Defaults to False.
        """
        self.file = file
        self.line_nb = line_nb
        self.with_wrapper = with_wrapper
        self.with_env_var = with_env_var

    def __str__(self):
        message = ""
        if not self.with_wrapper:
            message += "Please use the dedicated `fetch_secret.(sh|ps1)`."
        if not self.with_env_var:
            message += " Save your parameter name as environment variable in .gitlab-ci.yml file."
        return f"{self.file}:{self.line_nb + 1}. {message}"

    def __repr__(self):
        return str(self)


def list_get_parameter_calls(file):
    aws_ssm_call = re.compile(r"^.+ssm get-parameter.+--name +(?P<param>[^ ]+).*$")
    # remove the first letter of the script name because '\f' is badly interpreted for windows paths
    wrapper_call = re.compile(r"^.+etch_secret.(sh|ps1)[\"]? (-parameterName )?+(?P<param>[^ )]+).*$")
    calls = []
    with open(file) as f:
        try:
            for nb, line in enumerate(f):
                m = aws_ssm_call.match(line.strip())
                if m:
                    # Remove possible quotes
                    param = m["param"].replace('"', '').replace("'", "")
                    calls.append(
                        SSMParameterCall(file, nb, with_env_var=(param.startswith("$") or "os.environ" in param))
                    )
                m = wrapper_call.match(line.strip())
                param = m["param"].replace('"', '').replace("'", "") if m else None
                if m and not (param.startswith("$") or "os.environ" in param):
                    calls.append(SSMParameterCall(file, nb, with_wrapper=True))
        except UnicodeDecodeError:
            pass
    return calls


def get_gitlab_ci_lintable_jobs(diff_file, config_file, only_names=False):
    """Retrieves the jobs from full gitlab ci configuration file or from a diff file.

    Args:
        diff_file: Path to the diff file used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config.
        config_file: Path to the full gitlab ci configuration file obtained by compute-gitlab-ci-config.
    """

    assert (
        diff_file or config_file and not (diff_file and config_file)
    ), "You must provide either a diff file or a config file and not both"

    # Load all the jobs from the files
    if config_file:
        with open(config_file) as f:
            full_config = yaml.safe_load(f)
            jobs = [
                (job, job_contents)
                for contents in full_config.values()
                for job, job_contents in contents.items()
                if is_leaf_job(job, job_contents)
            ]
    else:
        with open(diff_file) as f:
            diff = MultiGitlabCIDiff.from_dict(yaml.safe_load(f))

        full_config = diff.after
        jobs = [(job, contents) for _, job, contents, _ in diff.iter_jobs(added=True, modified=True, only_leaves=True)]

    if not jobs:
        print(f"{color_message('Info', Color.BLUE)}: No added / modified jobs, skipping lint")
        return [], {}

    if only_names:
        jobs = [job for job, _ in jobs]

    return jobs, full_config


def _gitlab_ci_jobs_owners_lint(jobs, jobowners, ci_linters_config, path_jobowners):
    error_jobs = []
    n_ignored = 0
    for job in jobs:
        owners = [name for (kind, name) in jobowners.of(job) if kind == 'TEAM']
        if not owners:
            if job in ci_linters_config.job_owners_jobs:
                n_ignored += 1
            else:
                error_jobs.append(job)

    if n_ignored:
        print(
            f'{color_message("Info", Color.BLUE)}: {n_ignored} ignored jobs (jobs defined in {ci_linters_config.path}:job-owners)'
        )

    if error_jobs:
        error_jobs = '\n'.join(f'- {job}' for job in sorted(error_jobs))
        raise Exit(
            f"{color_message('Error', Color.RED)}: These jobs are not defined in {path_jobowners}:\n{error_jobs}"
        )
    else:
        print(f'{color_message("Success", Color.GREEN)}: All jobs have owners defined in {path_jobowners}')


def _gitlab_ci_jobs_codeowners_lint(path_codeowners, modified_yml_files, gitlab_owners):
    error_files = []
    for path in modified_yml_files:
        teams = [team for kind, team in gitlab_owners.of(path) if kind == 'TEAM']
        if not teams:
            error_files.append(path)

    if error_files:
        error_files = '\n'.join(f'- {path}' for path in sorted(error_files))

        raise Exit(
            f"{color_message('Error', Color.RED)}: These files should have specific CODEOWNERS rules within {path_codeowners} starting with '/.gitlab/<stage_name>'):\n{error_files}"
        )
    else:
        print(f'{color_message("Success", Color.GREEN)}: All files have CODEOWNERS rules within {path_codeowners}')
