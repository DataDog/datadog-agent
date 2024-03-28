import os
import platform
import subprocess

import gitlab
from gitlab.v4.objects import Project
from invoke.exceptions import Exit


BASE_URL = "https://gitlab.ddbuild.io"


def get_gitlab_token():
    if "GITLAB_TOKEN" not in os.environ:
        print("GITLAB_TOKEN not found in env. Trying keychain...")
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
    return os.environ["GITLAB_TOKEN"]


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
        print(
            "Please make sure that the GITLAB_BOT_TOKEN is set or that " "the GITLAB_BOT_TOKEN keychain entry is set."
        )
        raise Exit(code=1)
    return os.environ["GITLAB_BOT_TOKEN"]


def get_gitlab_api(token=None) -> gitlab.Gitlab:
    """
    Returns the gitlab api object with the api token.
    The token is the one of get_gitlab_token() by default.
    """
    token = token or get_gitlab_token()

    return gitlab.Gitlab(BASE_URL, private_token=token)


def get_gitlab_repo(repo='DataDog/datadog-agent', token=None) -> Project:
    api = get_gitlab_api(token)
    repo = api.projects.get(repo)

    return repo


# TODO
from invoke.tasks import task


@task
def test_gitlab(_):
    from tasks.libs.common.gitlab_api import get_gitlab_api, get_gitlab_repo
    from tasks.libs.pipeline_tools import (
        cancel_pipelines_with_confirmation,
        gracefully_cancel_pipeline,
        get_running_pipelines_on_same_ref,
    )

    api = get_gitlab_api()
    repo = get_gitlab_repo()
    ref = 'celian/gitlab-use-module-acix-65'

    # branch = 'celian/gitlab-use-module-acix-65'
    # api = Gitlab(api_token=get_gitlab_token())
    # api.test_project_found()
    # # print('CREATE PIPELINE', api.create_pipeline(branch))

    # print(api.all_pipelines_for_ref(branch))
    # print(api.last_pipeline_for_ref(branch))
    # from tasks.libs.pipeline_data import get_failed_jobs
    # print('failed', get_failed_jobs('DataDog/datadog-agent', '30533054'))
    # from tasks.libs.pipeline_notifications import get_failed_tests

    # # ok 30750076
    # # fail 30533054
    # job = get_gitlab_repo().jobs.get(469284993).asdict()
    # print(get_failed_tests('DataDog/datadog-agent', job))

    # --- Create / cancel pipeline ---
    # Get
    # pipeline = repo.pipelines.get(30899745)
    # pipeline = repo.pipelines.get(30634678)
    pipeline = repo.pipelines.get(30818003)
    print(pipeline)
    print(pipeline.web_url)
    # # Create
    # pipeline = repo.pipelines.create({'ref': 'celian/gitlab-use-module-acix-65'})
    # print(f'Pipeline {repo.web_url}/pipelines/{pipeline.id}')
    # # Cancel
    # pipelines = [30899006, 30899063]
    # pipelines = [repo.pipelines.get(n) for n in pipelines]
    # cancel_pipelines_with_confirmation(repo, pipelines)
    # Cancel jobs
    # gracefully_cancel_pipeline(repo, pipeline, ['source_test'])
    # pipeline.cancel()
    # Query
    # print('pipelines:', get_running_pipelines_on_same_ref(repo, ref))
    # Failed jobs
    # from tasks.notify import get_failed_jobs, get_failed_jobs_stats
    # print(get_failed_jobs('DataDog/datadog-agent', pipeline.id))
    # print(get_failed_jobs_stats('DataDog/datadog-agent', pipeline.id))
    # RC
    # from tasks.release import build_rc
    # from invoke.context import Context
    # build_rc(Context())
    # --- Kmt --
    # from tasks.kmt import gen_config_from_ci_pipeline
    # gen_config_from_ci_pipeline(_, pipeline=pipeline.id)
