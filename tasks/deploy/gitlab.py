import errno
import json
import os
import re
import sys
from urllib.parse import quote

errno_regex = re.compile(r".*\[Errno (\d+)\] (.*)")

__all__ = ["Gitlab"]

GITLAB_ERROR_BANNER = (
    "This could mean that you're not connected to Appgate " "or that GitLab is down (check https://gitlab.ddbuild.io)."
)


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
        print("You can make a request for access here: https://datadog.freshservice.com/support/catalog/items/63")
        sys.exit(1)

    def project(self, project_name):
        path = "/projects/%s" % quote(project_name, safe="")
        return self.make_request(path, json=True)

    def create_pipeline(self, project_name, ref, variables=None):
        """ref must be a branch or tag, not a commit."""
        if variables is None:
            variables = {}

        path = "/projects/%s/pipeline" % quote(project_name, safe="")
        headers = {"Content-Type": "application/json"}
        data = json.dumps({"ref": ref, "variables": [{"key": k, "value": v} for (k, v) in variables.items()],})
        return self.make_request(path, headers=headers, data=data, json=True)

    def pipelines_for_ref(self, project_name, ref, per_page=100):
        path = "/projects/%s/pipelines?ref=%s&per_page=%d" % (
            quote(project_name, safe=""),
            quote(ref, safe=""),
            per_page,
        )
        return self.make_request(path, json=True)

    def pipeline(self, project_name, pipeline_id):
        path = "/projects/{}/pipelines/{}".format(quote(project_name, safe=""), pipeline_id)
        return self.make_request(path, json=True)

    def jobs(self, project_name, pipeline_id, page=1, per_page=100):
        path = "/projects/{}/pipelines/{}/jobs?per_page={}&page={}".format(
            quote(project_name, safe=""), pipeline_id, per_page, page
        )
        return self.make_request(path, json=True)

    def trace(self, project_name, job_id):
        path = "/projects/%s/jobs/%d/trace" % (quote(project_name, safe=""), job_id)
        resp = self.make_request(path)
        return resp.read()

    def make_request(self, path, headers=None, data=None, json=False):
        import requests

        url = self.BASE_URL + path

        headers = dict(headers or [])
        headers["PRIVATE-TOKEN"] = self.api_token
        try:
            if data:
                r = requests.post(url, headers=headers, data=data)
            else:
                r = requests.get(url, headers=headers)
            if r.status_code == 401:
                print(
                    "HTTP 401: Your GITLAB_TOKEN may have expired. You can "
                    "check and refresh it at "
                    "https://gitlab.ddbuild.io/profile/personal_access_tokens"
                )
                print("Gitlab says: {}".format(r.json()["error_description"]))
                sys.exit(1)
        except requests.exceptions.Timeout:
            print("Connection to GitLab ({}) timed out.\n{}".format(url, GITLAB_ERROR_BANNER))
            sys.exit(1)
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
            sys.exit(1)
        if json:
            return r.json()
        return r.text

    def _api_token(self):
        if "GITLAB_TOKEN" not in os.environ:
            print(
                "Please create an 'api' access token at "
                "https://gitlab.ddbuild.io/profile/personal_access_tokens and "
                "export it is as GITLAB_TOKEN from your .bashrc or equivalent."
            )
            sys.exit(1)
        return os.environ["GITLAB_TOKEN"]

    def _trigger_token(self, project_name):
        env_var = "GITLAB_TRIGGER_TOKEN_" + project_name.upper().replace("-", "_").replace("/", "_")
        if env_var not in os.environ:
            print(
                "Please create a trigger at "
                "https://gitlab.ddbuild.io/%s/settings/ci_cd "
                "under 'Pipeline triggers', and export it is as %s from your "
                ".bashrc or equivalent." % (project_name, env_var)
            )
            print("Unfortunately this is only accessible to project admins for " "now.")
            sys.exit(1)
        return os.environ[env_var]
