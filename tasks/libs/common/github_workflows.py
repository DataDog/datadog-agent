import json
import os

from .githubapp import GithubApp, GithubAppException
from .remote_api import RemoteAPI

__all__ = ["GithubWorkflows", "GithubException", "get_github_app_token"]


class GithubException(Exception):
    pass


class GithubWorkflows(RemoteAPI):
    """
    Helper class to perform API calls against the Github Workflows API, using a Github App.
    """

    BASE_URL = "https://api.github.com"

    def __init__(self, repository="", api_token="", api_token_expiration_date=""):
        super(GithubWorkflows, self).__init__("GitHub Workflows")
        self.api_token = api_token
        self.api_token_expiration_date = api_token_expiration_date
        self.repository = repository
        self.authorization_error_message = (
            "HTTP 401: The token is invalid. Is the Github App still allowed to perform this action?"
        )

    def repo(self):
        """
        Gets the repo info.
        """

        path = f"/repos/{self.repository}"
        return self.make_request(path, method="GET", json_output=True)

    def trigger_workflow(self, workflow_name, ref, inputs=None):
        """
        Create a pipeline targeting a given reference of a project.
        ref must be a branch or a tag.
        """
        if inputs is None:
            inputs = dict()

        path = f"/repos/{self.repository}/actions/workflows/{workflow_name}/dispatches"
        data = json.dumps({"ref": ref, "inputs": inputs})
        return self.make_request(path, method="POST", data=data)

    def workflow_run(self, run_id):
        """
        Gets info on a specific workflow.
        """
        path = f"/repos/{self.repository}/actions/runs/{run_id}"
        return self.make_request(path, method="GET", json_output=True)

    def download_artifact(self, artifact_id, destination_dir):
        """
        Downloads the artifact identified by artifact_id to destination_dir.
        """
        path = f"/repos/{self.repository}/actions/artifacts/{artifact_id}/zip"
        content = self.make_request(path, method="GET", raw_output=True)

        zip_target_path = os.path.join(destination_dir, f"{artifact_id}.zip")
        with open(zip_target_path, "wb") as f:
            f.write(content)
        return zip_target_path

    def workflow_run_artifacts(self, run_id):
        """
        Gets list of artifacts for a workflow run.
        """
        path = f"/repos/{self.repository}/actions/runs/{run_id}/artifacts"
        return self.make_request(path, method="GET", json_output=True)

    def latest_workflow_run_for_ref(self, workflow_name, ref):
        """
        Gets latest workflow run for a given reference
        """
        runs = self.workflow_runs(workflow_name)
        ref_runs = [run for run in runs["workflow_runs"] if run["head_branch"] == ref]
        return max(ref_runs, key=lambda run: run['created_at'], default=None)

    def workflow_runs(self, workflow_name):
        """
        Gets all workflow runs for a workflow.
        """
        path = f"/repos/{self.repository}/actions/workflows/{workflow_name}/runs"
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
        headers["Authorization"] = f"token {self.api_token}"
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
        raise GithubException(f"Failed while making HTTP request: {method} {url}")


def get_github_app_token():
    try:
        token = GithubApp().get_token()
    except GithubAppException:
        raise GithubException("Couldn't get API token.")

    return token
