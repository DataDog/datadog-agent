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
