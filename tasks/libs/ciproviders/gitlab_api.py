from __future__ import annotations

import json
import os
import platform
import subprocess
from functools import lru_cache

import gitlab
import yaml
from gitlab.v4.objects import Project, ProjectPipeline
from invoke.exceptions import Exit

from tasks.libs.common.utils import retry_function

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

    return gitlab.Gitlab(BASE_URL, private_token=token, retry_transient_errors=True)


def get_gitlab_repo(repo='DataDog/datadog-agent', token=None) -> Project:
    api = get_gitlab_api(token)
    repo = api.projects.get(repo)

    return repo


@retry_function('refresh pipeline #{0.id}')
def refresh_pipeline(pipeline: ProjectPipeline):
    """
    Refresh a pipeline, retries if there is an error
    """
    pipeline.refresh()


class ConfigNodeList(list):
    """
    Wrapper of list to allow hashing and lru cache
    """

    def __init__(self, *args, **kwargs) -> None:
        self.extend(*args, **kwargs)

    def __hash__(self) -> int:
        return id(self)


class YamlReferenceTagList(ConfigNodeList):
    pass


class ConfigNodeDict(dict):
    """
    Wrapper of dict to allow hashing and lru cache
    """

    def __init__(self, *args, **kwargs) -> None:
        self.update(*args, **kwargs)

    def __hash__(self) -> int:
        return id(self)


class ReferenceTag(yaml.YAMLObject):
    """
    Custom yaml tag to handle references in gitlab-ci configuration
    """

    yaml_tag = '!reference'

    def __init__(self, references):
        self.references = references

    @classmethod
    def from_yaml(cls, loader, node):
        return YamlReferenceTagList(loader.construct_sequence(node))

    @classmethod
    def to_yaml(cls, dumper, data):
        return dumper.represent_sequence(cls.yaml_tag, data, flow_style=True)


def convert_to_config_node(json_data):
    """
    Convert json data to ConfigNode
    """
    if isinstance(json_data, dict):
        return ConfigNodeDict({k: convert_to_config_node(v) for k, v in json_data.items()})
    elif isinstance(json_data, list):
        constructor = YamlReferenceTagList if isinstance(json_data, YamlReferenceTagList) else ConfigNodeList

        return constructor([convert_to_config_node(v) for v in json_data])
    else:
        return json_data


def apply_yaml_extends(config: dict, node):
    """
    Applies `extends` yaml tags to the node and its children inplace

    > Example:
    Config:
    ```yaml
    .parent:
        hello: world
    node:
        extends: .parent
    ```

    apply_yaml_extends(node) updates node to:
    ```yaml
    node:
        hello: world
    ```
    """
    # Ensure node is an object that can contain extends
    if not isinstance(node, dict):
        return

    if 'extends' in node:
        parents = node['extends']
        if isinstance(parents, str):
            parents = [parents]

        # Merge parent
        for parent_name in parents:
            parent = config[parent_name]
            apply_yaml_postprocessing(config, parent)
            for key, value in parent.items():
                if key not in node:
                    node[key] = value

        del node['extends']


def apply_yaml_reference(config: dict, node):
    """
    Applies `!reference` gitlab yaml tags to the node and its children inplace

    > Example:
    Config:
    ```yaml
    .colors:
        - red
        - green
        - blue
    node:
        colors: !reference [.colors]
    ```

    apply_yaml_extends(node) updates node to:
    ```yaml
    node:
        colors:
            - red
            - green
            - blue
    ```
    """

    def apply_ref(value):
        """
        Applies reference tags
        """
        if isinstance(value, YamlReferenceTagList):
            assert value != [], 'Empty reference tag'

            # !reference [a, b, c] means we are looking for config[a][b][c]
            ref_value = config[value[0]]
            for i in range(1, len(value)):
                ref_value = ref_value[value[i]]

            apply_yaml_postprocessing(config, ref_value)

            return ref_value
        else:
            apply_yaml_postprocessing(config, value)

            return value

    if isinstance(node, dict):
        for key, value in node.items():
            node[key] = apply_ref(value)
    elif isinstance(node, list):
        for i, value in enumerate(node):
            node[i] = apply_ref(value)


@lru_cache(maxsize=None)
def apply_yaml_postprocessing(config: ConfigNodeDict, node):
    if isinstance(node, dict):
        for value in node.values():
            apply_yaml_postprocessing(config, value)
    elif isinstance(node, list):
        for value in node:
            apply_yaml_postprocessing(config, value)

    apply_yaml_extends(config, node)
    apply_yaml_reference(config, node)


def generate_gitlab_full_configuration(
    input_file, context=None, compare_to=None, return_dump=True, apply_postprocessing=False
):
    """
    Generate a full gitlab-ci configuration by resolving all includes

    - input_file: Initial gitlab yaml file (.gitlab-ci.yml)
    - context: Gitlab variables
    - compare_to: Override compare_to on change rules
    - return_dump: Whether to return the string dump or the dict object representing the configuration
    - apply_postprocessing: Whether or not to solve `extends` and `!reference` tags
    """
    # Update loader/dumper to handle !reference tag
    yaml.SafeLoader.add_constructor(ReferenceTag.yaml_tag, ReferenceTag.from_yaml)
    yaml.SafeDumper.add_representer(YamlReferenceTagList, ReferenceTag.to_yaml)

    full_configuration = read_includes(input_file, return_config=True)

    # Override some variables with a dedicated context
    if context:
        full_configuration["variables"].update(context)
    if compare_to:
        for value in full_configuration.values():
            if (
                isinstance(value, dict)
                and "changes" in value
                and isinstance(value["changes"], dict)
                and "compare_to" in value["changes"]
            ):
                value["changes"]["compare_to"] = compare_to
            elif isinstance(value, list):
                for v in value:
                    if (
                        isinstance(v, dict)
                        and "changes" in v
                        and isinstance(v["changes"], dict)
                        and "compare_to" in v["changes"]
                    ):
                        v["changes"]["compare_to"] = compare_to

    if apply_postprocessing:
        # We have to use ConfigNode to allow hashing and lru cache
        full_configuration = convert_to_config_node(full_configuration)
        apply_yaml_postprocessing(full_configuration, full_configuration)

    return yaml.safe_dump(full_configuration) if return_dump else full_configuration


def read_includes(yaml_files, includes=None, return_config=False, add_file_path=False):
    """
    Recursive method to read all includes from yaml files and store them in a list
    - add_file_path: add the file path to each object of the parsed file
    """
    if includes is None:
        includes = []

    if isinstance(yaml_files, str):
        yaml_files = [yaml_files]

    for yaml_file in yaml_files:
        current_file = read_content(yaml_file)

        if add_file_path:
            for value in current_file.values():
                if isinstance(value, dict):
                    value['_file_path'] = yaml_file

        if 'include' not in current_file:
            includes.append(current_file)
        else:
            read_includes(current_file['include'], includes, add_file_path=add_file_path)
            del current_file['include']
            includes.append(current_file)

    # Merge all files
    if return_config:
        full_configuration = {}
        for yaml_file in includes:
            full_configuration.update(yaml_file)

        return full_configuration


def read_content(file_path):
    """
    Read the content of a file, either from a local file or from an http endpoint
    """
    content = None
    if file_path.startswith('http'):
        import requests

        response = requests.get(file_path)
        response.raise_for_status()
        content = response.text
    else:
        with open(file_path) as f:
            content = f.read()
    return yaml.safe_load(content)


def get_preset_contexts(required_tests):
    possible_tests = ["all", "main", "release", "mq", "conductor"]
    required_tests = required_tests.casefold().split(",")
    if set(required_tests) | set(possible_tests) != set(possible_tests):
        raise Exit(f"Invalid test required: {required_tests} must contain only values from {possible_tests}", 1)
    main_contexts = [
        ("BUCKET_BRANCH", ["nightly"]),  # ["dev", "nightly", "beta", "stable", "oldnightly"]
        ("CI_COMMIT_BRANCH", ["main"]),  # ["main", "mq-working-branch-main", "7.42.x", "any/name"]
        ("CI_COMMIT_TAG", [""]),  # ["", "1.2.3-rc.4", "6.6.6"]
        ("CI_PIPELINE_SOURCE", ["pipeline"]),  # ["trigger", "pipeline", "schedule"]
        ("DEPLOY_AGENT", ["true"]),
        ("RUN_ALL_BUILDS", ["true"]),
        ("RUN_E2E_TESTS", ["auto"]),
        ("RUN_KMT_TESTS", ["on"]),
        ("RUN_UNIT_TESTS", ["on"]),
        ("TESTING_CLEANUP", ["true"]),
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
        ("TESTING_CLEANUP", ["true"]),
    ]
    mq_contexts = [
        ("BUCKET_BRANCH", ["dev"]),
        ("CI_COMMIT_BRANCH", ["mq-working-branch-main"]),
        ("CI_PIPELINE_SOURCE", ["pipeline"]),
        ("DEPLOY_AGENT", ["false"]),
        ("RUN_ALL_BUILDS", ["false"]),
        ("RUN_E2E_TESTS", ["auto"]),
        ("RUN_KMT_TESTS", ["off"]),
        ("RUN_UNIT_TESTS", ["off"]),
        ("TESTING_CLEANUP", ["false"]),
    ]
    conductor_contexts = [
        ("BUCKET_BRANCH", ["nightly"]),  # ["dev", "nightly", "beta", "stable", "oldnightly"]
        ("CI_COMMIT_BRANCH", ["main"]),  # ["main", "mq-working-branch-main", "7.42.x", "any/name"]
        ("CI_COMMIT_TAG", [""]),  # ["", "1.2.3-rc.4", "6.6.6"]
        ("CI_PIPELINE_SOURCE", ["pipeline"]),  # ["trigger", "pipeline", "schedule"]
        ("DDR_WORKFLOW_ID", ["true"]),
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
    """
    Load a context either from a yaml file or from a json string
    """
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
