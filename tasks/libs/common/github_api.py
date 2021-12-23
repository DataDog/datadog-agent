import json
import os
import platform
import subprocess

from invoke.exceptions import Exit

from .remote_api import RemoteAPI

__all__ = ["GithubAPI", "get_github_token"]


class GithubAPI(RemoteAPI):
    """
    Helper class to perform API calls against the Github API, using a Github PAT.
    """

    BASE_URL = "https://api.github.com"

    def __init__(self, repository="", api_token=""):
        super(GithubAPI, self).__init__("GitHub API")
        self.api_token = api_token
        self.repository = repository
        self.authorization_error_message = (
            "HTTP 401: The token is invalid. Is the Github token provided still allowed to perform this action?"
        )

    def repo(self):
        """
        Gets the repo info.
        """

        path = f"/repos/{self.repository}"
        return self.make_request(path, method="GET", json_output=True)

    def get_branch(self, branch_name):
        """
        Gets info on a given branch in the given Github repository.
        """

        path = f"/repos/{self.repository}/branches/{branch_name}"
        return self.make_request(path, method="GET", json_output=True)

    def create_pr(self, pr_title, pr_body, base_branch, target_branch):
        """
        Creates a PR in the given Github repository.
        """

        path = f"/repos/{self.repository}/pulls"
        data = json.dumps({"head": target_branch, "base": base_branch, "title": pr_title, "body": pr_body})
        return self.make_request(path, method="POST", json_output=True, data=data)

    def update_pr(self, pull_number, milestone_number, labels):
        """
        Updates a given PR with the provided milestone number and labels.
        """

        path = f"/repos/{self.repository}/issues/{pull_number}"
        data = json.dumps(
            {
                "milestone": milestone_number,
                "labels": labels,
            }
        )
        return self.make_request(path, method="POST", json_output=True, data=data)

    def get_milestone_by_name(self, milestone_name):
        """
        Searches for a milestone in the given repository that matches the provided name,
        and returns data about it.
        """
        path = f"/repos/{self.repository}/milestones"
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
        headers["Authorization"] = f"token {self.api_token}"
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


def get_github_token():
    if "GITHUB_TOKEN" not in os.environ:
        print("GITHUB_TOKEN not found in env. Trying keychain...")
        if platform.system() == "Darwin":
            try:
                output = subprocess.check_output(
                    ['security', 'find-generic-password', '-a', os.environ["USER"], '-s', 'GITHUB_TOKEN', '-w']
                )
                if output:
                    return output.strip()
            except subprocess.CalledProcessError:
                print("GITHUB_TOKEN not found in keychain...")
                pass
        raise Exit(
            message="Please create a 'repo' access token at "
            "https://github.com/settings/tokens and "
            "add it as GITHUB_TOKEN in your keychain "
            "or export it from your .bashrc or equivalent.",
            code=1,
        )
    return os.environ["GITHUB_TOKEN"]
