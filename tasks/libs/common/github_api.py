import base64
import os
import platform
import re
import subprocess

try:
    from github import Auth, Github, GithubException, GithubIntegration, GithubObject
except ImportError:
    # PyGithub isn't available on some build images, ignore it for now
    # and fail hard if it gets used.
    pass
from invoke.exceptions import Exit

__all__ = ["GithubAPI"]

errno_regex = re.compile(r".*\[Errno (\d+)\] (.*)")


class GithubAPI:
    """
    Helper class to perform API calls against the Github API, using a Github PAT.
    """

    BASE_URL = "https://api.github.com"

    def __init__(self, repository=""):
        self._auth = self._chose_auth()
        self._github = Github(auth=self._auth)
        self._repository = self._github.get_repo(repository)

    def repo(self):
        """
        Gets the repo info.
        """

        return self._repository

    def get_branch(self, branch_name):
        """
        Gets info on a given branch in the given Github repository.
        """
        try:
            return self._repository.get_branch(branch_name)
        except GithubException as e:
            if e.status == 404:
                return None
            raise e

    def create_pr(self, pr_title, pr_body, base_branch, target_branch):
        """
        Creates a PR in the given Github repository.
        """
        return self._repository.create_pull(title=pr_title, body=pr_body, base=base_branch, target=target_branch)

    def update_pr(self, pull_number, milestone_number, labels):
        """
        Updates a given PR with the provided milestone number and labels.
        """
        pr = self._repository.get_pull(pull_number)
        milestone = self._repository.get_milestone(milestone_number)
        issue = pr.as_issue()
        issue.edit(milestone=milestone, labels=labels)
        return pr

    def get_milestone_by_name(self, milestone_name):
        """
        Searches for a milestone in the given repository that matches the provided name,
        and returns data about it.
        """
        milestones = self._repository.get_milestones()
        for milestone in milestones:
            if milestone.title == milestone_name:
                return milestone
        return None

    def get_pulls(self, milestone=None, labels=None):
        if milestone is None:
            m = GithubObject.NotSet
        else:
            m = self.get_milestone_by_name(milestone)
            if not m:
                print(f'Unknown milestone {milestone}')
                return None
        if labels is None:
            labels = []
        issues = self._repository.get_issues(milestone=m, state='all', labels=labels)
        return [i.as_pull_request() for i in issues if i.pull_request is not None]

    def get_tags(self, pattern=""):
        """
        List all tags starting with the provided pattern.
        If the pattern is empty or None, all tags will be returned
        """
        tags = self._repository.get_tags()
        if pattern is None or len(pattern) == 0:
            return list(tags)
        return [t for t in tags if t.name.startswith(pattern)]

    def trigger_workflow(self, workflow_name, ref, inputs=None):
        """
        Create a pipeline targeting a given reference of a project.
        ref must be a branch or a tag.
        """
        workflow = self._repository.get_workflow(workflow_name)
        if workflow is None:
            return False
        if inputs is None:
            inputs = dict()
        return workflow.create_dispatch(ref, inputs)

    def workflow_run(self, run_id):
        """
        Gets info on a specific workflow.
        """
        return self._repository.get_workflow_run(run_id)

    def download_artifact(self, artifact, destination_dir):
        """
        Downloads the artifact identified by artifact_id to destination_dir.
        """
        url = artifact.archive_download_url

        return self.download_from_url(url, destination_dir, destination_file=artifact.id)

    def download_from_url(self, url, destination_dir, destination_file):
        import requests

        headers = {
            "Authorization": f'{self._auth.token_type} {self._auth.token}',
            "Accept": "application/vnd.github.v3+json",
        }
        # Retrying this request if needed is handled by the caller
        with requests.get(url, headers=headers, stream=True) as r:
            r.raise_for_status()
            zip_target_path = os.path.join(destination_dir, f"{destination_file}.zip")
            with open(zip_target_path, "wb") as f:
                for chunk in r.iter_content(chunk_size=8192):
                    f.write(chunk)
        return zip_target_path

    def download_logs(self, run_id, destination_dir):

        run = self._repository.get_workflow_run(run_id)
        logs_url = run.logs_url
        _, headers, _ = run._requester.requestJson("GET", logs_url)

        return self.download_from_url(headers["location"], destination_dir, run.id)

    def workflow_run_for_ref_after_date(self, workflow_name, ref, oldest_date):
        """
        Gets all the workflow triggered after a given date
        """
        workflow = self._repository.get_workflow(workflow_name)
        runs = workflow.get_runs(branch=ref)
        recent_runs = [run for run in runs if run.created_at > oldest_date]

        return sorted(recent_runs, key=lambda run: run.created_at, reverse=True)

    def _chose_auth(self):
        """
        Attempt to find a working authentication, in order:
            - Personal access token through GITHUB_TOKEN environment variable
            - An app token through the GITHUB_APP_ID & GITHUB_KEY_B64 environment
              variables (can also use GITHUB_INSTALLATION_ID to save a request)
            - A token from macOS keychain
        """
        if "GITHUB_TOKEN" in os.environ:
            return Auth.Token(os.environ["GITHUB_TOKEN"])
        if "GITHUB_APP_ID" in os.environ and "GITHUB_KEY_B64" in os.environ:
            appAuth = Auth.AppAuth(
                os.environ['GITHUB_APP_ID'], base64.b64decode(os.environ['GITHUB_KEY_B64']).decode('ascii')
            )
            installation_id = os.environ.get('GITHUB_INSTALLATION_ID', None)
            if installation_id is None:
                # Even if we don't know the installation id, there's an API endpoint to
                # retrieve it, given the other credentials (app id + key).
                integration = GithubIntegration(auth=appAuth)
                installations = integration.get_installations()
                if len(installations) == 0:
                    raise Exit(message='No usable installation found', code=1)
                installation_id = installations[0]
            return appAuth.get_installation_auth(int(installation_id))
        if platform.system() == "Darwin":
            try:
                output = (
                    subprocess.check_output(
                        ['security', 'find-generic-password', '-a', os.environ["USER"], '-s', 'GITHUB_TOKEN', '-w']
                    )
                    .decode()
                    .strip()
                )

                if output:
                    return Auth.Token(output)
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
