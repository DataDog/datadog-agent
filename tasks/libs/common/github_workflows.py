import json
import os

from .githubapp import GithubApp, GithubAppException
from .remote_api import RemoteAPI

__all__ = ["GithubWorkflows", "GithubException"]


class GithubException(Exception):
    pass


class GithubWorkflows(RemoteAPI):
    BASE_URL = "https://api.github.com"

    def __init__(self, api_token=None):
        self.api_token = api_token if api_token else self._api_token()
        self.api_name = "GitHub Workflows"
        self.authorization_error_message = (
            "HTTP 401: The token is invalid. Is the Github App still allowed to perform this action?"
        )

    def repo(self, repo_name):
        """
        Gets the repo info.
        """

        path = "/repos/{}".format(repo_name)
        return self.make_request(path, method="GET", json_output=True)

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
        return self.make_request(path, method="GET", json_output=True)

    def download_artifact(self, repo_name, artifact_id, destination_dir):
        """
        Downloads the artifact identified by artifact_id to destination_dir.
        """
        path = "/repos/{}/actions/artifacts/{}/zip".format(repo_name, artifact_id)
        content = self.make_request(path, method="GET", raw_output=True)

        zip_target_path = os.path.join(destination_dir, "{}.zip".format(artifact_id))
        with open(zip_target_path, "wb") as f:
            f.write(content)
        return zip_target_path

    def workflow_run_artifacts(self, repo_name, run_id):
        """
        Gets list of artifacts for a workflow run.
        """
        path = "/repos/{}/actions/runs/{}/artifacts".format(repo_name, run_id)
        return self.make_request(path, method="GET", json_output=True)

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
        return self.make_request(path, method="GET", json_output=True)

    def make_request(self, path, headers=None, method="GET", data=None, json_output=False, raw_output=False):
        """
        Utility to make an HTTP request to the GitHub API.
        See RemoteAPI#request.

        Adds "Authorization: token {self.api_token}" and "Accept: application/vnd.github.v3+json"
        to the headers to be able to authenticate ourselves to GitHub.
        """
        url = self.BASE_URL + path

        headers = dict(headers or [])
        headers["Authorization"] = "token {}".format(self.api_token)
        headers["Accept"] = "application/vnd.github.v3+json"

        for _ in range(5):  # Retry up to 5 times
            return self.request(
                path=path,
                headers=headers,
                data=data,
                json_input=False,
                json_output=json_output,
                raw_output=raw_output,
                stream_output=False,
                method=method,
            )
        raise GithubException("Failed while making HTTP request: {} {}".format(method, url))

    def _api_token(self):
        try:
            token = GithubApp().get_token()
        except GithubAppException:
            raise GithubException("Couldn't get API token.")

        return token
