import errno
import json
import os
import re

from .githubapp import GithubApp, GithubAppException

errno_regex = re.compile(r".*\[Errno (\d+)\] (.*)")

__all__ = ["Github", "GithubException"]


class GithubException(Exception):
    pass


class Github(object):
    BASE_URL = "https://api.github.com"

    def __init__(self, api_token=None):
        self.api_token = api_token if api_token else self._api_token()

    def repo(self, repo_name):
        """
        Gets the repo info.
        """

        path = "/repos/{}".format(repo_name)
        return self.make_request(path, method="GET", output_format="json")

    def trigger_workflow(self, repo_name, workflow_name, ref, inputs=None):
        """
        Create a pipeline targeting a given reference of a project.
        ref must be a branch or a tag.
        """
        if inputs is None:
            inputs = dict()

        path = "/repos/{}/actions/workflows/{}/dispatches".format(repo_name, workflow_name)
        data = json.dumps({"ref": ref, "inputs": inputs})
        return self.make_request(path, method="POST", data=data)

    def workflow_run(self, repo_name, run_id):
        """
        Gets info on a specific workflow.
        """
        path = "/repos/{}/actions/runs/{}".format(repo_name, run_id)
        return self.make_request(path, method="GET", output_format="json")

    def download_artifact(self, repo_name, artifact_id, destination_dir):
        """
        Downloads the artifact identified by artifact_id to destination_dir.
        """
        path = "/repos/{}/actions/artifacts/{}/zip".format(repo_name, artifact_id)
        content = self.make_request(path, method="GET", output_format="raw")

        zip_target_path = os.path.join(destination_dir, "{}.zip".format(artifact_id))
        with open(zip_target_path, "wb") as f:
            f.write(content)
        return zip_target_path

    def workflow_run_artifacts(self, repo_name, run_id):
        """
        Gets list of artifacts for a workflow run.
        """
        path = "/repos/{}/actions/runs/{}/artifacts".format(repo_name, run_id)
        return self.make_request(path, method="GET", output_format="json")

    def latest_workflow_run_for_ref(self, repo_name, workflow_name, ref):
        """
        Gets latest workflow run for a given reference
        """
        runs = self.workflow_runs(repo_name, workflow_name)
        ref_runs = [run for run in runs["workflow_runs"] if run["head_branch"] == ref]
        return max(ref_runs, key=lambda run: run['created_at'], default=None)

    def workflow_runs(self, repo_name, workflow_name):
        """
        Gets all workflow runs for a workflow.
        """
        path = "/repos/{}/actions/workflows/{}/runs".format(repo_name, workflow_name)
        return self.make_request(path, method="GET", output_format="json")

    def make_request(self, endpoint, headers=None, method="GET", data=None, output_format="text"):
        """
        Utility to make an HTTP request to the Gitlab API.
        
        endpoint is the HTTP endpoint that will be requested.

        headers is a dict of HTTP headers that can be added to the request.

        Adds "Authorization: token {self.api_token}" and "Accept: application/vnd.github.v3+json"
        to the headers to be able to authenticate ourselves to Github.

        The method parameter dictates the type of request made (GET or POST).
        If method is GET, the data parameter is ignored (no body can be sent in a GET request).
        
        The output_format allows changing the structure of the response:
        - text: a string containing the body of the response.
        - json: an object containing the deserialized json body response. Works only if the response
                is a json object.
        - raw: a binary blob. Mainly useful when downloading things.
        """
        import requests

        url = self.BASE_URL + endpoint

        headers = dict(headers or [])
        headers["Authorization"] = "token {}".format(self.api_token)
        headers["Accept"] = "application/vnd.github.v3+json"
        for _ in range(5):  # Retry up to 5 times
            try:
                if method == 'GET':
                    r = requests.get(url, headers=headers)
                if method == 'POST':
                    if data:
                        r = requests.post(url, headers=headers, data=data)
                    else:
                        r = requests.post(url, headers=headers)
                if r.status_code < 400:  # Success
                    if output_format == "json":
                        return r.json()
                    if output_format == "raw":
                        return r.content
                    return r.text
                if r.status_code == 401:
                    print("HTTP 401: The token is invalid. Is the Github App still allowed to perform this action?")
                    print("Github says: {}".format(r.json()["error_description"]))
            except requests.exceptions.Timeout:
                print("Connection to Github ({}) timed out.".format(url))
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
                    print("Connection to Github ({}) refused".format(url))
                else:
                    print("Error while connecting to {}: {}".format(url, str(e)))
        raise GithubException("Failed while making HTTP request: {} {}".format(method, url))

    def _api_token(self):
        try:
            token = GithubApp().get_token()
        except GithubAppException:
            raise GithubException("Couldn't get API token.")

        return token
