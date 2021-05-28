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
        return self.make_request(path, json=True)

    def create_pipeline(self, project_name, ref, variables=None):
        """
        Create a pipeline targeting a given reference of a project.
        ref must be a branch or a tag.
        """
        if variables is None:
            variables = {}

        path = "/projects/{}/pipeline".format(quote(project_name, safe=""))
        headers = {"Content-Type": "application/json"}
        data = json.dumps({"ref": ref, "variables": [{"key": k, "value": v} for (k, v) in variables.items()],})
        return self.make_request(path, headers=headers, data=data, json=True)

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
        return self.make_request(path, json=True)

    def last_pipeline_for_ref(self, project_name, ref, per_page=100):
        """
        Gets the last pipeline for a given reference.
        per_page cannot exceed 100.
        """
        pipelines = self.pipelines_for_ref(project_name, ref, per_page=per_page)

        if len(pipelines) == 0:
            return None

        return sorted(pipelines, key=lambda pipeline: pipeline['created_at'], reverse=True)[0]

    def pipeline(self, project_name, pipeline_id):
        """
        Gets info for a given pipeline.
        """
        path = "/projects/{}/pipelines/{}".format(quote(project_name, safe=""), pipeline_id)
        return self.make_request(path, json=True)

    def cancel_pipeline(self, project_name, pipeline_id):
        """
        Cancels a given pipeline.
        """
        path = "/projects/{}/pipelines/{}/cancel".format(quote(project_name, safe=""), pipeline_id)
        return self.make_request(path, json=True, method="POST")

    def commit(self, project_name, commit_sha):
        """
        Gets info for a given commit sha.
        """
        path = "/projects/{}/repository/commits/{}".format(quote(project_name, safe=""), commit_sha)
        return self.make_request(path, json=True)

    def artifact(self, project_name, job_id, artifact_name):
        path = "/projects/{}/jobs/{}/artifacts/{}".format(quote(project_name, safe=""), job_id, artifact_name)
        response = self.make_request(path, stream=True)
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
        return self.make_request(path, json=True)

    def find_tag(self, project_name, tag_name):
        """
        Look up a tag by its name.
        """
        path = "/projects/{}/repository/tags/{}".format(quote(project_name, safe=""), tag_name)
        return self.make_request(path, json=True)

    def make_request(self, path, headers=None, data=None, json=False, stream=False, method=None):
        """
        Utility to make a request to the Gitlab API.
        """
        import requests

        url = self.BASE_URL + path

        headers = dict(headers or [])
        headers["PRIVATE-TOKEN"] = self.api_token

        # TODO: Use the param argument of requests instead of handling URL params
        # manually
        try:
            if data or method == "POST":
                r = requests.post(url, headers=headers, data=data, stream=stream)
            else:
                r = requests.get(url, headers=headers, stream=stream)
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
        if json:
            return r.json()
        if stream:
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
