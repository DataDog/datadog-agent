import json
import os
import platform
import subprocess

from invoke.exceptions import Exit

from .remote_api import RemoteAPI

__all__ = ["GithubAPI"]


class GithubAPI(RemoteAPI):
    BASE_URL = "https://api.github.com"

    def __init__(self, api_token=None):
        self.api_token = api_token if api_token else self._api_token()

    def repo(self, repo_name):
        """
        Gets the repo info.
        """

        path = "/repos/{}".format(repo_name)
        return self.make_request(path, method="GET", json_output=True)

    def get_branch(self, repo_name, branch_name):
        """
        Creates a PR in the given repository.
        """

        path = "/repos/{}/branches/{}".format(repo_name, branch_name)
        return self.make_request(path, method="GET", json_output=True)

    def create_pr(self, repo_name, pr_title, pr_body, base_branch, target_branch):
        """
        Creates a PR in the given repository.
        """

        path = "/repos/{}/pulls".format(repo_name)
        data = json.dumps({"head": target_branch, "base": base_branch, "title": pr_title, "body": pr_body})
        return self.make_request(path, method="POST", json_output=True, data=data)

    def update_pr(self, repo_name, pull_number, milestone, labels):
        path = "/repos/{}/issues/{}".format(repo_name, pull_number)
        data = json.dumps(
            {
                "milestone": milestone,
                "labels": labels,
            }
        )
        return self.make_request(path, method="POST", json_output=True, data=data)

    def get_milestone_by_name(self, repo_name, milestone_name):
        path = "/repos/{}/milestones".format(repo_name)
        res = self.make_request(path, method="GET", json_output=True)
        for milestone in res:
            if milestone["title"] == milestone_name:
                return milestone
        return None

    def make_request(self, path, headers=None, method="GET", data=None, json_output=False):
        """
        Utility to make an HTTP request to the GitHub API.
        See RemoteAPI#request.

        Adds "Authorization: token {self.api_token}" and "Accept: application/vnd.github.v3+json"
        to the headers to be able to authenticate ourselves to GitHub.
        """
        headers = dict(headers or [])
        headers["Authorization"] = "token {}".format(self.api_token)
        headers["Accept"] = "application/vnd.github.v3+json"

        return self.request(
            path=path,
            headers=headers,
            data=data,
            json_input=False,
            json_output=json_output,
            stream_output=False,
            method=method,
        )

    def _api_token(self):
        if "GITHUB_TOKEN" not in os.environ:
            print("GITHUB_TOKEN not found in env. Trying keychain...")
            if platform.system() == "Darwin":
                try:
                    output = subprocess.check_output(
                        ['security', 'find-generic-password', '-a', os.environ["USER"], '-s', 'GITHUB_TOKEN', '-w']
                    )
                    if len(output) > 0:
                        return output.strip()
                except subprocess.CalledProcessError:
                    print("GITHUB_TOKEN not found in keychain...")
                    pass
            print(
                "Please create a 'repo' access token at "
                "https://github.com/settings/tokens and "
                "add it as GITHUB_TOKEN in your keychain "
                "or export it from your .bashrc or equivalent."
            )
            raise Exit(code=1)
        return os.environ["GITHUB_TOKEN"]
