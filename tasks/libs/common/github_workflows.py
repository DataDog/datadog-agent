import datetime
import json
import os
import random
import string
import time

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
        # generate a random id
        run_id = ''.join(random.choices(string.ascii_uppercase + string.digits, k=15))
        # filter runs that were created after this date minus 5 minutes
        delta_time = datetime.timedelta(minutes=5)
        run_date_filter = (datetime.datetime.utcnow() - delta_time).strftime("%Y-%m-%dT%H:%M")
        if inputs is None:
            inputs = {id: run_id}
        else:
            inputs["id"] = run_id

        path = f"/repos/{self.repository}/actions/workflows/{workflow_name}/dispatches"
        data = json.dumps({"ref": ref, "inputs": inputs})
        self.make_request(path, method="POST", data=data)

        workflow_id = ""
        try_number = 0
        while workflow_id == "" and try_number < 10:
            runs = self.workflow_runs(workflow_name, f"?created=%3E{run_date_filter}")
            ref_runs = [run for run in runs["workflow_runs"] if run["head_branch"] == ref]
            if len(runs) > 0:
                for workflow in ref_runs:
                    jobs_url = workflow["jobs_url"]
                    print(f"get jobs_url {jobs_url}")
                    jobs = self.make_request(jobs_url, method="GET", json_output=True)
                    if len(jobs) > 0:
                        # we only take the first job
                        job = jobs[0]
                        steps = job["steps"]
                        if len(steps) >= 2:
                            second_step = steps[0]  # run_id is at first position
                            if second_step["name"] == run_id:
                                workflow_id = job["run_id"]
                        else:
                            print("waiting for steps to be executed...")
                            time.sleep(3)
                    else:
                        print("waiting for jobs to popup...")
                        time.sleep(3)
            else:
                print("waiting for workflows to popup...")
                time.sleep(3)
            try_number += 1
        return workflow_id

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

    def workflow_runs(self, workflow_name, filter=None):
        """
        Gets all workflow runs for a workflow.
        """
        path = f"/repos/{self.repository}/actions/workflows/{workflow_name}/runs"
        if filter is not None:
            path += f"{filter}"
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
