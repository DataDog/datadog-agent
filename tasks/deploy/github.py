import errno
import json
import os
import re

from invoke.exceptions import Exit

from .githubapp import GithubApp

errno_regex = re.compile(r".*\[Errno (\d+)\] (.*)")

__all__ = ["Github"]


class Github(object):
    BASE_URL = "https://api.github.com"

    def __init__(self, api_token=None):
        self.api_token = api_token if api_token else self._api_token()

    def test_repo_found(self, repo):
        """
        Checks if a repo can be found. This is useful for testing access permissions to repos.
        """
        result = self.repo(repo)

        # name is arbitrary, just need to check if something is in the result
        if "name" in result:
            return

        print("Cannot find Github repo {}".format(repo))
        print("If you cannot see it in the Github UI, you likely need permission.")
        raise Exit(code=1)

    def repo(self, repo_name):
        """
        Gets the repo info.
        """

        path = "/repos/{}".format(repo_name)
        return self.make_request(path, json=True)

    def trigger_workflow(self, repo_name, workflow_name, ref, inputs=None):
        """
        Create a pipeline targeting a given reference of a project.
        ref must be a branch or a tag.
        """
        if inputs is None:
            inputs = {}

        path = "/repos/{}/actions/workflows/{}/dispatches".format(repo_name, workflow_name)
        data = json.dumps({"ref": ref, "inputs": inputs})
        return self.make_request(path, data=data)

    def workflow_run(self, repo_name, run_id):
        """
        Gets info on a specific workflow
        """
        path = "/repos/{}/actions/runs/{}".format(repo_name, run_id)
        return self.make_request(path, json=True)

    def download_artifact(self, repo_name, artifact_id, destination_dir):
        """
        Gets info on a specific workflow
        """
        path = "/repos/{}/actions/artifacts/{}/zip".format(repo_name, artifact_id)
        content = self.make_request(path, raw_content=True)

        zip_target_path = os.path.join(destination_dir, "{}.zip".format(artifact_id))
        with open(zip_target_path, "wb") as f:
            f.write(content)
        return zip_target_path

    def workflow_run_artifacts(self, repo_name, run_id):
        """
        Gets list of artifacts for a run
        """
        path = "/repos/{}/actions/runs/{}/artifacts".format(repo_name, run_id)
        return self.make_request(path, json=True)

    def latest_workflow_run_for_ref(self, repo_name, workflow_name, ref):
        """
        Gets latest workflow run for a given reference
        """
        runs = self.workflow_runs(repo_name, workflow_name)
        ref_runs = [run for run in runs["workflow_runs"] if run["head_branch"] == ref]

        if len(ref_runs) == 0:
            return None

        return sorted(ref_runs, key=lambda run: run['created_at'], reverse=True)[0]

    def workflow_runs(self, repo_name, workflow_name):
        """
        Gets all workflow runs for a workflow.
        """
        path = "/repos/{}/actions/workflows/{}/runs".format(repo_name, workflow_name)
        return self.make_request(path, json=True)

    def make_request(self, path, headers=None, data=None, json=False, raw_content=False):
        """
        Utility to make a request to the Gitlab API.
        """
        import requests

        url = self.BASE_URL + path

        print(url)

        headers = dict(headers or [])
        headers["Authorization"] = "token {}".format(self.api_token)
        headers["Accept"] = "application/vnd.github.v3+json"
        try:
            if data:
                r = requests.post(url, headers=headers, data=data)
            else:
                r = requests.get(url, headers=headers)
            if r.status_code == 401:
                print(
                    "HTTP 401: Your GITHUB_TOKEN may have expired. You can "
                    "check and refresh it at "
                    "https://github.com/settings/tokens"
                )
                print("Github says: {}".format(r.json()["error_description"]))
                raise Exit(code=1)
        except requests.exceptions.Timeout:
            print("Connection to Github ({}) timed out.".format(url))
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
                print("Connection to Github ({}) refused".format(url))
            else:
                print("Error while connecting to {}: {}".format(url, str(e)))
            raise Exit(code=1)
        if json:
            return r.json()
        if raw_content:
            return r.content
        return r.text

    def _api_token(self):
        return GithubApp().get_token()
