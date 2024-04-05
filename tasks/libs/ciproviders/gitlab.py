import json
import os
import platform
import subprocess
from collections import UserList
from urllib.parse import quote

import yaml
from invoke.exceptions import Exit

from tasks.libs.common.remote_api import APIError, RemoteAPI

__all__ = ["Gitlab"]


class Gitlab(RemoteAPI):
    """
    Helper class to perform API calls against the Gitlab API, using a Gitlab PAT.
    """

    BASE_URL = "https://gitlab.ddbuild.io/api/v4"

    def __init__(self, project_name="DataDog/datadog-agent", api_token=""):
        super(Gitlab, self).__init__("Gitlab")
        self.api_token = api_token
        self.project_name = project_name
        self.authorization_error_message = (
            "HTTP 401: Your GITLAB_TOKEN may have expired. You can "
            "check and refresh it at "
            "https://gitlab.ddbuild.io/-/profile/personal_access_tokens"
        )

    def test_project_found(self):
        """
        Checks if a project can be found. This is useful for testing access permissions to projects.
        """
        result = self.project()

        # name is arbitrary, just need to check if something is in the result
        if "name" in result:
            return

        print(f"Cannot find GitLab project {self.project_name}")
        print("If you cannot see it in the GitLab WebUI, you likely need permission.")
        raise Exit(code=1)

    def project(self):
        """
        Gets the project info.
        """
        path = f"/projects/{quote(self.project_name, safe='')}"
        return self.make_request(path, json_output=True)

    def create_pipeline(self, ref, variables=None):
        """
        Create a pipeline targeting a given reference of a project.
        ref must be a branch or a tag.
        """
        if variables is None:
            variables = {}

        path = f"/projects/{quote(self.project_name, safe='')}/pipeline"
        headers = {"Content-Type": "application/json"}
        data = json.dumps({"ref": ref, "variables": [{"key": k, "value": v} for (k, v) in variables.items()]})
        return self.make_request(path, headers=headers, data=data, json_output=True)

    def all_pipelines_for_ref(self, ref, sha=None):
        """
        Gets all pipelines for a given reference (+ optionally git sha).
        """
        page = 1

        # Go through all pages
        results = self.pipelines_for_ref(ref, sha=sha, page=page)
        while results:
            yield from results
            page += 1
            results = self.pipelines_for_ref(ref, sha=sha, page=page)

    def pipelines_for_ref(self, ref, sha=None, page=1, per_page=100):
        """
        Gets one page of pipelines for a given reference (+ optionally git sha).
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipelines?ref={quote(ref, safe='')}&per_page={per_page}&page={page}"
        if sha:
            path = f"{path}&sha={sha}"
        return self.make_request(path, json_output=True)

    def last_pipeline_for_ref(self, ref, per_page=100):
        """
        Gets the last pipeline for a given reference.
        per_page cannot exceed 100.
        """
        pipelines = self.pipelines_for_ref(ref, per_page=per_page)

        if len(pipelines) == 0:
            return None

        return sorted(pipelines, key=lambda pipeline: pipeline['created_at'], reverse=True)[0]

    def last_pipelines(self):
        """
        Get the last 100 pipelines
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipelines?per_page=100&page=1"
        return self.make_request(path, json_output=True)

    def trigger_pipeline(self, data):
        """
        Trigger a pipeline on a project using the trigger endpoint.
        Requires a trigger token in the data object, in the 'token' field.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/trigger/pipeline"

        if 'token' not in data:
            raise Exit("Missing 'token' field in data object to trigger child pipelines", 1)

        return self.make_request(path, data=data, json_input=True, json_output=True)

    def pipeline(self, pipeline_id):
        """
        Gets info for a given pipeline.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipelines/{pipeline_id}"
        return self.make_request(path, json_output=True)

    def cancel_pipeline(self, pipeline_id):
        """
        Cancels a given pipeline.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipelines/{pipeline_id}/cancel"
        return self.make_request(path, json_output=True, method="POST")

    def cancel_job(self, job_id):
        """
        Cancels a given job
        """
        path = f"/projects/{quote(self.project_name, safe='')}/jobs/{job_id}/cancel"
        return self.make_request(path, json_output=True, method="POST")

    def commit(self, commit_sha):
        """
        Gets info for a given commit sha.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/repository/commits/{commit_sha}"
        return self.make_request(path, json_output=True)

    def artifact(self, job_id, artifact_name, ignore_not_found=False):
        path = f"/projects/{quote(self.project_name, safe='')}/jobs/{job_id}/artifacts/{artifact_name}"
        try:
            response = self.make_request(path, stream_output=True)
            return response
        except APIError as e:
            if e.status_code == 404 and ignore_not_found:
                return None
            raise e

    def all_jobs(self, pipeline_id):
        """
        Gets all the jobs for a pipeline.
        """
        page = 1

        # Go through all pages
        results = self.jobs(pipeline_id, page)
        while results:
            yield from results
            page += 1
            results = self.jobs(pipeline_id, page)

    def jobs(self, pipeline_id, page=1, per_page=100):
        """
        Gets one page of the jobs for a pipeline.
        per_page cannot exceed 100.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipelines/{pipeline_id}/jobs?per_page={per_page}&page={page}"
        return self.make_request(path, json_output=True)

    def job_log(self, job_id):
        """
        Gets the log file for a given job.
        """

        path = f"/projects/{quote(self.project_name, safe='')}/jobs/{job_id}/trace"
        return self.make_request(path)

    def all_pipeline_schedules(self):
        """
        Gets all pipelines schedules for the given project.
        """
        page = 1

        # Go through all pages
        results = self.pipeline_schedules(page)
        while results:
            yield from results
            page += 1
            results = self.pipeline_schedules(page)

    def pipeline_schedules(self, page=1, per_page=100):
        """
        Gets one page of the pipeline schedules for the given project.
        per_page cannot exceed 100
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules?per_page={per_page}&page={page}"
        return self.make_request(path, json_output=True)

    def pipeline_schedule(self, schedule_id):
        """
        Gets a single pipeline schedule.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules/{schedule_id}"
        return self.make_request(path, json_output=True)

    def create_pipeline_schedule(self, description, ref, cron, cron_timezone=None, active=None):
        """
        Create a new pipeline schedule with given attributes.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules"
        data = {
            "description": description,
            "ref": ref,
            "cron": cron,
            "cron_timezone": cron_timezone,
            "active": active,
        }
        no_none_data = {k: v for k, v in data.items() if v is not None}
        return self.make_request(path, data=no_none_data, json_output=True, json_input=True)

    def edit_pipeline_schedule(
        self, schedule_id, description=None, ref=None, cron=None, cron_timezone=None, active=None
    ):
        """
        Edit an existing pipeline schedule with given attributes.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules/{schedule_id}"
        data = {
            "description": description,
            "ref": ref,
            "cron": cron,
            "cron_timezone": cron_timezone,
            "active": active,
        }
        no_none_data = {k: v for k, v in data.items() if v is not None}
        return self.make_request(path, json_input=True, json_output=True, data=no_none_data, method="PUT")

    def delete_pipeline_schedule(self, schedule_id):
        """
        Delete an existing pipeline schedule.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules/{schedule_id}"
        # Gitlab API docs claim that this returns the JSON representation of the deleted schedule,
        # but it actually returns an empty string
        result = self.make_request(path, json_output=False, method="DELETE")
        return f"Pipeline schedule deleted; result: {result if result else '(empty)'}"

    def create_pipeline_schedule_variable(self, schedule_id, key, value):
        """
        Create a variable for an existing pipeline schedule.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules/{schedule_id}/variables"
        data = {
            "key": key,
            "value": value,
        }
        return self.make_request(path, data=data, json_output=True, json_input=True)

    def edit_pipeline_schedule_variable(self, schedule_id, key, value):
        """
        Edit an existing variable for a pipeline schedule.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules/{schedule_id}/variables/{key}"
        return self.make_request(path, json_input=True, data={"value": value}, json_output=True, method="PUT")

    def delete_pipeline_schedule_variable(self, schedule_id, key):
        """
        Delete an existing variable for a pipeline schedule.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules/{schedule_id}/variables/{key}"
        return self.make_request(path, json_output=True, method="DELETE")

    def find_tag(self, tag_name):
        """
        Look up a tag by its name.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/repository/tags/{tag_name}"
        try:
            response = self.make_request(path, json_output=True)
            return response
        except APIError as e:
            # If Gitlab API returns a "404 not found" error we return an empty dict
            if e.status_code == 404:
                print(
                    f"Couldn't find the {tag_name} tag: Gitlab returned a 404 Not Found instead of a 200 empty response."
                )
                return dict()
            else:
                raise e

    def lint(self, configuration):
        """
        Lint a gitlab-ci configuration.
        """
        path = f"/projects/{quote(self.project_name, safe='')}/ci/lint?dry_run=true&include_jobs=true"
        headers = {"Content-Type": "application/json"}
        data = {"content": configuration}
        return self.make_request(path, headers=headers, data=data, json_input=True, json_output=True)

    def make_request(
        self, path, headers=None, data=None, json_input=False, json_output=False, stream_output=False, method=None
    ):
        """
        Utility to make a request to the Gitlab API.
        See RemoteAPI#request.

        Adds "PRIVATE-TOKEN: {self.api_token}" to the headers to be able to authenticate ourselves to GitLab.
        """
        headers = dict(headers or [])
        headers["PRIVATE-TOKEN"] = self.api_token

        return self.request(
            path=path,
            headers=headers,
            data=data,
            json_input=json_input,
            json_output=json_output,
            stream_output=stream_output,
            raw_output=False,
            method=method,
        )


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


def generate_gitlab_full_configuration(input_file, context=None):
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
    # Override some variables with a dedicated context
    if context:
        full_configuration["variables"].update(context)
    return yaml.safe_dump(full_configuration)


def read_includes(yaml_file, includes):
    """
    Recursive method to read all includes from yaml files and store them in a list
    """
    current_file = read_content(yaml_file)
    if 'include' not in current_file:
        includes.append(current_file)
    else:
        for include in current_file['include']:
            read_includes(include, includes)
        del current_file['include']
        includes.append(current_file)


def read_content(file_path):
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


def get_preset_contexts(test):
    tests = ["all", "main", "release", "mq"]
    if test not in tests:
        raise Exit(f"Invalid test type: {test}, must belong to {tests}", 1)
    main_contexts = [
        ("BUCKET_BRANCH", ["nightly", "stable"]),  # ["dev", "nightly", "beta", "stable", "oldnightly"]
        ("CI_COMMIT_BRANCH", ["main"]),  # ["main", "mq-working-branch-main", "7.42.x", "any/name"]
        ("CI_COMMIT_TAG", [""]),  # ["", "1.2.3-rc.4", "6.6.6"]
        ("CI_PIPELINE_SOURCE", ["pipeline"]),  # ["trigger", "pipeline", "schedule"]
        ("DEPLOY_AGENT", ["true", "false"]),
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
        ("CI_PIPELINE_SOURCE", ["trigger", "schedule"]),
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
    all_contexts = []
    if test in ["all", "main"]:
        generate_contexts(main_contexts, [], all_contexts)
    if test in ["all", "release"]:
        generate_contexts(release_contexts, [], all_contexts)
    if test in ["all", "mq"]:
        generate_contexts(mq_contexts, [], all_contexts)
    return all_contexts


def generate_contexts(contexts, context, all_contexts):
    if len(contexts) == 0:
        all_contexts.append(context[:])
        return
    for value in contexts[0][1]:
        context.append((contexts[0][0], value))
        generate_contexts(contexts[1:], context, all_contexts)
        context.pop()


def retrieve_context(context):
    if os.path.exists(context):
        with open(context) as f:
            y = yaml.safe_load(f)
        if "variables" not in y:
            raise Exit(
                f"Invalid context file: {context}, missing 'variables' key. Input file must be similar to tasks/unit-tests/testdata/gitlab_main_context_template.yml",
                1,
            )
        return [[(k, v) for k, v in y["variables"].items()]]
    else:
        try:
            j = json.loads(context)
            return [[(k, v) for k, v in j.items()]]
        except json.JSONDecodeError:
            raise Exit(f"Invalid context: {context}, must be a valid json, or a path to a yaml file", 1)
