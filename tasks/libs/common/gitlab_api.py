import json
import os
import platform
import subprocess
from urllib.parse import quote

import gitlab
from gitlab.v4.objects import Project
from invoke.exceptions import Exit

from tasks.libs.common.remote_api import APIError, RemoteAPI

BASE_URL = "https://gitlab.ddbuild.io"


# class Gitlab(RemoteAPI):
#     """
#     Helper class to perform API calls against the Gitlab API, using a Gitlab PAT.
#     """

#     # BASE_URL = "https://gitlab.ddbuild.io"

#     def __init__(self, project_name="DataDog/datadog-agent", api_token=None):
#         super(Gitlab, self).__init__("Gitlab")
#         raise RuntimeError('Deprecated API used')
#         # self.api_token = api_token
#         self.project_name = project_name
#         # self.authorization_error_message = (
#         #     "HTTP 401: Your GITLAB_TOKEN may have expired. You can "
#         #     "check and refresh it at "
#         #     "https://gitlab.ddbuild.io/-/profile/personal_access_tokens"
#         # )
#         self.gl = gitlab.Gitlab(self.BASE_URL, private_token=api_token)

#     def test_project_found(self):
#         """
#         Checks if a project can be found. This is useful for testing access permissions to projects.
#         """
#         try:
#             self.project()
#         except gitlab.GitlabError:
#             print(f"Cannot find GitLab project {self.project_name}")
#             print("If you cannot see it in the GitLab WebUI, you likely need permission.")
#             raise Exit(code=1)

#     def project(self):
#         """
#         Gets the project info.
#         """
#         return self.gl.projects.get(self.project_name)

#     def create_pipeline(self, ref, variables=None):
#         """
#         Create a pipeline targeting a given reference of a project.
#         ref must be a branch or a tag.
#         """
#         if variables is None:
#             variables = {}

#         # path = f"/projects/{quote(self.project_name, safe='')}/pipeline"
#         # headers = {"Content-Type": "application/json"}
#         # data = json.dumps()
#         # return self.make_request(path, headers=headers, data=data, json_output=True)

#         repo = self.project()
#         pipeline = repo.pipelines.create(
#             {"ref": ref, "variables": [{"key": k, "value": v} for (k, v) in variables.items()]}
#         )

#         return pipeline

#     def all_pipelines_for_ref(self, ref, sha=None):
#         """
#         Gets all pipelines for a given reference (+ optionally git sha).
#         """
#         # page = 1

#         # # Go through all pages
#         # results = self.pipelines_for_ref(ref, sha=sha, page=page)
#         # while results:
#         #     yield from results
#         #     page += 1
#         #     results = self.pipelines_for_ref(ref, sha=sha, page=page)

#         repo = self.project()

#         return repo.pipelines.list(ref=ref, sha=sha, all=True)

#     def pipelines_for_ref(self, ref, sha=None, page=1, per_page=100):
#         """
#         Gets one page of pipelines for a given reference (+ optionally git sha).
#         """
#         # path = f"/projects/{quote(self.project_name, safe='')}/pipelines?ref={quote(ref, safe='')}&per_page={per_page}&page={page}"
#         # if sha:
#         #     path = f"{path}&sha={sha}"
#         # return self.make_request(path, json_output=True)

#         repo = self.project()

#         return repo.pipelines.list(ref=ref, sha=sha, page=page, per_page=per_page)

#     def last_pipeline_for_ref(self, ref, per_page=100):
#         """
#         Gets the last pipeline for a given reference.
#         per_page cannot exceed 100.
#         """
#         pipelines = self.pipelines_for_ref(ref, per_page=per_page)

#         if len(pipelines) == 0:
#             return None

#         return sorted(pipelines, key=lambda pipeline: pipeline.asdict()['created_at'], reverse=True)[0]

#     def trigger_pipeline(self, data):
#         """
#         Trigger a pipeline on a project using the trigger endpoint.
#         Requires a trigger token in the data object, in the 'token' field.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/trigger/pipeline"

#         if 'token' not in data:
#             raise Exit("Missing 'token' field in data object to trigger child pipelines", 1)

#         return self.make_request(path, data=data, json_input=True, json_output=True)

#     def pipeline(self, pipeline_id):
#         """
#         Gets info for a given pipeline.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/pipelines/{pipeline_id}"
#         return self.make_request(path, json_output=True)

#     def cancel_pipeline(self, pipeline_id):
#         """
#         Cancels a given pipeline.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/pipelines/{pipeline_id}/cancel"
#         return self.make_request(path, json_output=True, method="POST")

#     def cancel_job(self, job_id):
#         """
#         Cancels a given job
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/jobs/{job_id}/cancel"
#         return self.make_request(path, json_output=True, method="POST")

#     def commit(self, commit_sha):
#         """
#         Gets info for a given commit sha.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/repository/commits/{commit_sha}"
#         return self.make_request(path, json_output=True)

#     def artifact(self, job_id, artifact_name, ignore_not_found=False):
#         path = f"/projects/{quote(self.project_name, safe='')}/jobs/{job_id}/artifacts/{artifact_name}"
#         try:
#             response = self.make_request(path, stream_output=True)
#             return response
#         except APIError as e:
#             if e.status_code == 404 and ignore_not_found:
#                 return None
#             raise e

#     def all_jobs(self, pipeline_id):
#         """
#         Gets all the jobs for a pipeline.
#         """
#         page = 1

#         # Go through all pages
#         results = self.jobs(pipeline_id, page)
#         while results:
#             yield from results
#             page += 1
#             results = self.jobs(pipeline_id, page)

#     def jobs(self, pipeline_id, page=1, per_page=100):
#         """
#         Gets one page of the jobs for a pipeline.
#         per_page cannot exceed 100.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/pipelines/{pipeline_id}/jobs?per_page={per_page}&page={page}"
#         return self.make_request(path, json_output=True)

#     def job_log(self, job_id):
#         """
#         Gets the log file for a given job.
#         """

#         path = f"/projects/{quote(self.project_name, safe='')}/jobs/{job_id}/trace"
#         return self.make_request(path)

#     def all_pipeline_schedules(self):
#         """
#         Gets all pipelines schedules for the given project.
#         """
#         page = 1

#         # Go through all pages
#         results = self.pipeline_schedules(page)
#         while results:
#             yield from results
#             page += 1
#             results = self.pipeline_schedules(page)

#     def pipeline_schedules(self, page=1, per_page=100):
#         """
#         Gets one page of the pipeline schedules for the given project.
#         per_page cannot exceed 100
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules?per_page={per_page}&page={page}"
#         return self.make_request(path, json_output=True)

#     def pipeline_schedule(self, schedule_id):
#         """
#         Gets a single pipeline schedule.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules/{schedule_id}"
#         return self.make_request(path, json_output=True)

#     def create_pipeline_schedule(self, description, ref, cron, cron_timezone=None, active=None):
#         """
#         Create a new pipeline schedule with given attributes.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules"
#         data = {
#             "description": description,
#             "ref": ref,
#             "cron": cron,
#             "cron_timezone": cron_timezone,
#             "active": active,
#         }
#         no_none_data = {k: v for k, v in data.items() if v is not None}
#         return self.make_request(path, data=no_none_data, json_output=True, json_input=True)

#     def edit_pipeline_schedule(
#         self, schedule_id, description=None, ref=None, cron=None, cron_timezone=None, active=None
#     ):
#         """
#         Edit an existing pipeline schedule with given attributes.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules/{schedule_id}"
#         data = {
#             "description": description,
#             "ref": ref,
#             "cron": cron,
#             "cron_timezone": cron_timezone,
#             "active": active,
#         }
#         no_none_data = {k: v for k, v in data.items() if v is not None}
#         return self.make_request(path, json_input=True, json_output=True, data=no_none_data, method="PUT")

#     def delete_pipeline_schedule(self, schedule_id):
#         """
#         Delete an existing pipeline schedule.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules/{schedule_id}"
#         # Gitlab API docs claim that this returns the JSON representation of the deleted schedule,
#         # but it actually returns an empty string
#         result = self.make_request(path, json_output=False, method="DELETE")
#         return f"Pipeline schedule deleted; result: {result if result else '(empty)'}"

#     def create_pipeline_schedule_variable(self, schedule_id, key, value):
#         """
#         Create a variable for an existing pipeline schedule.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules/{schedule_id}/variables"
#         data = {
#             "key": key,
#             "value": value,
#         }
#         return self.make_request(path, data=data, json_output=True, json_input=True)

#     def edit_pipeline_schedule_variable(self, schedule_id, key, value):
#         """
#         Edit an existing variable for a pipeline schedule.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules/{schedule_id}/variables/{key}"
#         return self.make_request(path, json_input=True, data={"value": value}, json_output=True, method="PUT")

#     def delete_pipeline_schedule_variable(self, schedule_id, key):
#         """
#         Delete an existing variable for a pipeline schedule.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/pipeline_schedules/{schedule_id}/variables/{key}"
#         return self.make_request(path, json_output=True, method="DELETE")

#     def find_tag(self, tag_name):
#         """
#         Look up a tag by its name.
#         """
#         path = f"/projects/{quote(self.project_name, safe='')}/repository/tags/{tag_name}"
#         try:
#             response = self.make_request(path, json_output=True)
#             return response
#         except APIError as e:
#             # If Gitlab API returns a "404 not found" error we return an empty dict
#             if e.status_code == 404:
#                 print(
#                     f"Couldn't find the {tag_name} tag: Gitlab returned a 404 Not Found instead of a 200 empty response."
#                 )
#                 return dict()
#             else:
#                 raise e

#     def make_request(
#         self, path, headers=None, data=None, json_input=False, json_output=False, stream_output=False, method=None
#     ):
#         """
#         Utility to make a request to the Gitlab API.
#         See RemoteAPI#request.

#         Adds "PRIVATE-TOKEN: {self.api_token}" to the headers to be able to authenticate ourselves to GitLab.
#         """
#         headers = dict(headers or [])
#         headers["PRIVATE-TOKEN"] = self.api_token

#         return self.request(
#             path=path,
#             headers=headers,
#             data=data,
#             json_input=json_input,
#             json_output=json_output,
#             stream_output=stream_output,
#             raw_output=False,
#             method=method,
#         )


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
    pipeline = repo.pipelines.get(30899745)
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
    from tasks.release import build_rc
    from invoke.context import Context
    build_rc(Context())
    # print(get_failed_jobs('DataDog/datadog-agent', pipeline.id))
    # print(get_failed_jobs_stats('DataDog/datadog-agent', pipeline.id))
