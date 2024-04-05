import os
import platform
import subprocess
from collections import UserList

import gitlab
import yaml
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


class ReferenceTag(yaml.YAMLObject):
    """
    Custom yaml tag to handle references in gitlab-ci configuration
    """

    yaml_tag = u'!reference'

    def __init__(self, references):
        self.references = references

    @classmethod
    def from_yaml(cls, loader, node):
        return UserList(loader.construct_sequence(node))

    @classmethod
    def to_yaml(cls, dumper, data):
        return dumper.represent_sequence(cls.yaml_tag, data.data, flow_style=True)


def generate_gitlab_full_configuration(input_file):
    """
    Generate a full gitlab-ci configuration by resolving all includes
    """
    # Update loader/dumper to handle !reference tag
    yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
    yaml.SafeDumper.add_representer(UserList, ReferenceTag.to_yaml)

    yaml_contents = []
    read_includes(input_file, yaml_contents)
    full_configuration = {}
    for yaml_file in yaml_contents:
        full_configuration.update(yaml_file)
    return yaml.safe_dump(full_configuration)


def read_includes(yaml_file, includes):
    """
    Recursive method to read all includes from yaml files and store them in a list
    """
    with open(yaml_file) as f:
        current_file = yaml.safe_load(f)
    if 'include' not in current_file:
        includes.append(current_file)
    else:
        for include in current_file['include']:
            if include.startswith('http'):
                continue
            read_includes(include, includes)
        del current_file['include']
        includes.append(current_file)
