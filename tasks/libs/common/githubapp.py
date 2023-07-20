import base64
import logging
import os
import time
from datetime import datetime

from .remote_api import RemoteAPI

logger = logging.getLogger(__name__)
logger.setLevel(os.environ.get('LOGGING_LEVEL', 'INFO'))


# Defining a custom exception class to throw if the requests error out
class GithubAppException(Exception):
    pass


class GithubApp(RemoteAPI):

    BASE_URL = 'https://api.github.com'

    def __init__(self):
        super(GithubApp, self).__init__("GitHub APP")
        self.key_b64 = os.environ['GITHUB_KEY_B64']
        self.app_id = os.environ['GITHUB_APP_ID']
        self.installation_id = os.environ.get('GITHUB_INSTALLATION_ID', None)
        if self.installation_id is None:
            # Even if we don't know the installation id, there's an API endpoint to
            # retrieve it, given the other credentials (app id + key).
            self.installation_id = self.get_installation()

    def gen_token_payload(self):
        """
        Generate the token needed to authenticate as a Github App.
        """
        # To get authenticated, you need to use a token with:
        # - iat (creation_time)
        # - exp (expiration time, here 5 minutes since we re-create a token every time we want to authenticate)
        # - iss (the Github app id)
        # and encode it with the private key that's associated with the app.
        # See: https://docs.github.com/en/developers/apps/authenticating-with-github-apps#authenticating-as-a-github-app
        epoch = int(time.time())
        payload = {'iat': epoch, 'exp': epoch + (5 * 60), 'iss': self.app_id}
        return payload

    def auth_headers(self):
        """
        Craft headers to make a request to the Github API as a Github App.
        """
        import jwt

        token_payload = self.gen_token_payload()
        bearer_token = jwt.encode(token_payload, base64.b64decode(self.key_b64), algorithm='RS256')
        headers = {
            'Authorization': f'Bearer {bearer_token}',
            'Accept': 'application/vnd.github.v3+json',
        }
        return headers

    def get_token(self):
        """
        Get installation token for a Github App, which then allows to access
        the Github API with the permissions of this App.
        """
        endpoint = f'/app/installations/{self.installation_id}/access_tokens'
        r = self.make_request(endpoint, method='POST', json_output=True)
        return r.get("token"), datetime.strptime(r.get("expires_at"), "%Y-%m-%dT%H:%M:%SZ")

    def get_installation(self):
        """
        List the installations of the Github App identified by self.app_id,
        and return the first one.

        HACK: we should check the entire list and take the "correct" installation
        for our usage, but as of now we expect the App to only have one installation.
        """
        endpoint = '/app/installations'
        r = self.make_request(endpoint, method='GET', json_output=True)
        return r[0]["id"]
