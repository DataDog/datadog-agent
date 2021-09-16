import errno
import json
import os
import platform
import re
import subprocess
from urllib.parse import quote

from invoke.exceptions import Exit

errno_regex = re.compile(r".*\[Errno (\d+)\] (.*)")

__all__ = ["Gitlab"]


class Gitlab(object):
    BASE_URL = "https://gitlab.ddbuild.io/api/v4"

    def __init__(self, api_token=None):
        self.api_token = api_token if api_token else self._api_token()

    def test_project_found(self, project):
        """
        Checks if a project can be found. This is useful for testing access permissions to projects.
        """
        result = self.project(project)

        # name is arbitrary, just need to check if something is in the result
        if "name" in result:
            return

        print("Cannot find GitLab project {}".format(project))
        print("If you cannot see it in the GitLab WebUI, you likely need permission.")
        raise Exit(code=1)

    def project(self, project_name):
        """
        Gets the project info.
        """
        path = "/projects/{}".format(quote(project_name, safe=""))
        return self.make_request(path, json_output=True)

    def create_pipeline(self, project_name, ref, variables=None):
        """
        Create a pipeline targeting a given reference of a project.
        ref must be a branch or a tag.
        """
        if variables is None:
            variables = {}

        path = "/projects/{}/pipeline".format(quote(project_name, safe=""))
        headers = {"Content-Type": "application/json"}
        data = json.dumps({"ref": ref, "variables": [{"key": k, "value": v} for (k, v) in variables.items()]})
        return self.make_request(path, headers=headers, data=data, json_output=True)

    def all_pipelines_for_ref(self, project_name, ref, sha=None):
        """
        Gets all pipelines for a given reference (+ optionally git sha).
        """
        page = 1

        # Go through all pages
        results = self.pipelines_for_ref(project_name, ref, sha=sha, page=page)
        while results:
            yield from results
            page += 1
            results = self.pipelines_for_ref(project_name, ref, sha=sha, page=page)

    def pipelines_for_ref(self, project_name, ref, sha=None, page=1, per_page=100):
        """
        Gets one page of pipelines for a given reference (+ optionally git sha).
        """
        path = "/projects/{}/pipelines?ref={}&per_page={}&page={}".format(
            quote(project_name, safe=""), quote(ref, safe=""), per_page, page
        )
        if sha:
            path = "{}&sha={}".format(path, sha)
        return self.make_request(path, json_output=True)

    def last_pipeline_for_ref(self, project_name, ref, per_page=100):
        """
        Gets the last pipeline for a given reference.
        per_page cannot exceed 100.
        """
        pipelines = self.pipelines_for_ref(project_name, ref, per_page=per_page)

        if len(pipelines) == 0:
            return None

        return sorted(pipelines, key=lambda pipeline: pipeline['created_at'], reverse=True)[0]

    def trigger_pipeline(self, project_name, data):
        """
        Trigger a pipeline on a project using the trigger endpoint.
        Requires a trigger token in the data object, in the 'token' field.
        """
        path = "/projects/{}/trigger/pipeline".format(quote(project_name, safe=""))

        if 'token' not in data:
            raise Exit("Missing 'token' field in data object to trigger child pipelines", 1)

        return self.make_request(path, data=data, json_input=True, json_output=True)

    def pipeline(self, project_name, pipeline_id):
        """
        Gets info for a given pipeline.
        """
        path = "/projects/{}/pipelines/{}".format(quote(project_name, safe=""), pipeline_id)
        return self.make_request(path, json_output=True)

    def cancel_pipeline(self, project_name, pipeline_id):
        """
        Cancels a given pipeline.
        """
        path = "/projects/{}/pipelines/{}/cancel".format(quote(project_name, safe=""), pipeline_id)
        return self.make_request(path, json_output=True, method="POST")

    def commit(self, project_name, commit_sha):
        """
        Gets info for a given commit sha.
        """
        path = "/projects/{}/repository/commits/{}".format(quote(project_name, safe=""), commit_sha)
        return self.make_request(path, json_output=True)

    def artifact(self, project_name, job_id, artifact_name):
        path = "/projects/{}/jobs/{}/artifacts/{}".format(quote(project_name, safe=""), job_id, artifact_name)
        response = self.make_request(path, stream_output=True)
        if response.status_code != 200:
            return None
        return response

    def all_jobs(self, project_name, pipeline_id):
        """
        Gets all the jobs for a pipeline.
        """
        page = 1

        # Go through all pages
        results = self.jobs(project_name, pipeline_id, page)
        while results:
            yield from results
            page += 1
            results = self.jobs(project_name, pipeline_id, page)

    def jobs(self, project_name, pipeline_id, page=1, per_page=100):
        """
        Gets one page of the jobs for a pipeline.
        per_page cannot exceed 100.
        """
        path = "/projects/{}/pipelines/{}/jobs?per_page={}&page={}".format(
            quote(project_name, safe=""), pipeline_id, per_page, page
        )
        return self.make_request(path, json_output=True)

    def all_pipeline_schedules(self, project_name):
        """
        Gets all pipelines schedules for the given project.
        """
        page = 1

        # Go through all pages
        results = self.pipeline_schedules(project_name, page)
        while results:
            yield from results
            page += 1
            results = self.pipeline_schedules(project_name, page)

    def pipeline_schedules(self, project_name, page=1, per_page=100):
        """
        Gets one page of the pipeline schedules for the given project.
        per_page cannot exceed 100
        """
        path = "/projects/{}/pipeline_schedules?per_page={}&page={}".format(
            quote(project_name, safe=""), per_page, page
        )
        return self.make_request(path, json_output=True)

    def pipeline_schedule(self, project_name, schedule_id):
        """
        Gets a single pipeline schedule.
        """
        path = "/projects/{}/pipeline_schedules/{}".format(quote(project_name, safe=""), schedule_id)
        return self.make_request(path, json_output=True)

    def create_pipeline_schedule(self, project_name, description, ref, cron, cron_timezone=None, active=None):
        """
        Create a new pipeline schedule with given attributes.
        """
        path = "/projects/{}/pipeline_schedules".format(quote(project_name, safe=""))
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
        self, project_name, schedule_id, description=None, ref=None, cron=None, cron_timezone=None, active=None
    ):
        """
        Edit an existing pipeline schedule with given attributes.
        """
        path = "/projects/{}/pipeline_schedules/{}".format(quote(project_name, safe=""), schedule_id)
        data = {
            "description": description,
            "ref": ref,
            "cron": cron,
            "cron_timezone": cron_timezone,
            "active": active,
        }
        no_none_data = {k: v for k, v in data.items() if v is not None}
        return self.make_request(path, json_output=True, data=no_none_data, method="PUT")

    def delete_pipeline_schedule(self, project_name, schedule_id):
        """
        Delete an existing pipeline schedule.
        """
        path = "/projects/{}/pipeline_schedules/{}".format(quote(project_name, safe=""), schedule_id)
        # Gitlab API docs claim that this returns the JSON representation of the deleted schedule,
        # but it actually returns an empty string
        result = self.make_request(path, json_output=False, method="DELETE")
        return "Pipeline schedule deleted; result: {}".format(result if result else "(empty)")

    def create_pipeline_schedule_variable(self, project_name, schedule_id, key, value):
        """
        Create a variable for an existing pipeline schedule.
        """
        path = "/projects/{}/pipeline_schedules/{}/variables".format(quote(project_name, safe=""), schedule_id)
        data = {
            "key": key,
            "value": value,
        }
        return self.make_request(path, data=data, json_output=True, json_input=True)

    def edit_pipeline_schedule_variable(self, project_name, schedule_id, key, value):
        """
        Edit an existing variable for a pipeline schedule.
        """
        path = "/projects/{}/pipeline_schedules/{}/variables/{}".format(quote(project_name, safe=""), schedule_id, key)
        return self.make_request(path, data={"value": value}, json_output=True, method="PUT")

    def delete_pipeline_schedule_variable(self, project_name, schedule_id, key):
        """
        Delete an existing variable for a pipeline schedule.
        """
        path = "/projects/{}/pipeline_schedules/{}/variables/{}".format(quote(project_name, safe=""), schedule_id, key)
        return self.make_request(path, json_output=True, method="DELETE")

    def find_tag(self, project_name, tag_name):
        """
        Look up a tag by its name.
        """
        path = "/projects/{}/repository/tags/{}".format(quote(project_name, safe=""), tag_name)
        return self.make_request(path, json_output=True)

    def make_request(
        self, path, headers=None, data=None, json_input=False, json_output=False, stream_output=False, method=None
    ):
        """
        Utility to make a request to the Gitlab API.

        headers: A hash of headers to pass to the request.
        data: An object containing the body of the request.
        json_input: If set to true, data is passed with the json parameter of requests.post instead of the data parameter.

        By default, the request method is GET, or POST if data is not empty.
        method: Can be set to "POST" to force a POST request even when data is empty.

        By default, we return the text field of the response object. The following fields can alter this behavior:
        json_output: the json field of the response object is returned.
        stream_output: the request asks for a stream response, and the raw response object is returned.
        """
        import requests

        url = self.BASE_URL + path

        headers = dict(headers or [])
        headers["PRIVATE-TOKEN"] = self.api_token

        # TODO: Use the param argument of requests instead of handling URL params
        # manually
        try:
            # If json_input is true, we specifically want to send data using the json
            # parameter of requests.post
            if data and json_input:
                r = requests.post(url, headers=headers, json=data, stream=stream_output)
            elif method == "PUT":
                r = requests.put(url, headers=headers, json=data, stream=stream_output)
            elif method == "DELETE":
                r = requests.delete(url, headers=headers, stream=stream_output)
            elif data or method == "POST":
                r = requests.post(url, headers=headers, data=data, stream=stream_output)
            else:
                r = requests.get(url, headers=headers, stream=stream_output)
            if r.status_code == 401:
                print(
                    "HTTP 401: Your GITLAB_TOKEN may have expired. You can "
                    "check and refresh it at "
                    "https://gitlab.ddbuild.io/profile/personal_access_tokens"
                )
                print("Gitlab says: {}".format(r.json()["error_description"]))
                raise Exit(code=1)
        except requests.exceptions.Timeout:
            print("Connection to GitLab ({}) timed out.".format(url))
            raise Exit(code=1)
        except requests.exceptions.RequestException as e:
            m = errno_regex.match(str(e))
            if not m:
                print("Unknown error raised connecting to {}: {}".format(url, e))

            # Parse errno to give a better explanation
            # Requests doesn't have granularity at the level we want:
            # http://docs.python-requests.org/en/master/_modules/requests/exceptions/
            errno_code = int(m.group(1))
            message = m.group(2)

            if errno_code == errno.ENOEXEC:
                print("Error resolving {}: {}".format(url, message))
            elif errno_code == errno.ECONNREFUSED:
                print("Connection to Gitlab ({}) refused".format(url))
            else:
                print("Error while connecting to {}: {}".format(url, str(e)))
            raise Exit(code=1)
        if json_output:
            return r.json()
        if stream_output:
            return r
        return r.text

    def _api_token(self):
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
                "https://gitlab.ddbuild.io/profile/personal_access_tokens and "
                "add it as GITLAB_TOKEN in your keychain "
                "or export it from your .bashrc or equivalent."
            )
            raise Exit(code=1)
        return os.environ["GITLAB_TOKEN"]
