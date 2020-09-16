import base64
import logging
import os
import time

# Defines how many times the HTTP requests will be retried
NB_RETRIES = 3

logger = logging.getLogger(__name__)
logger.setLevel(os.environ.get('LOGGING_LEVEL', 'INFO'))

# Defining a custom exception class to throw if the requests error out
class GithubException(Exception):
    pass


class GithubApp:
    def __init__(self):
        self.key_b64 = os.environ['GITHUB_KEY_B64']
        self.app_id = os.environ['GITHUB_APP_ID']
        self.base_url = 'https://api.github.com'
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

    def get_headers(self):
        """
        Craft headers to make a request to the Github API as a Github App.
        """
        import jwt

        token_payload = self.gen_token_payload()
        bearer_token = jwt.encode(token_payload, base64.b64decode(self.key_b64), algorithm='RS256').decode()
        headers = {
            'Authorization': 'Bearer {}'.format(bearer_token),
            'Accept': 'application/vnd.github.v3+json',
        }
        return headers

    def make_request(self, endpoint, method='GET', payload=None):
        """
        Make a request to the Github API, while using a token crafted from the App ID and
        private key to authenticate ourselves as a Github App.
        """
        import requests

        print(endpoint)
        if method == 'GET':
            return requests.get(
                '{base_url}{endpoint}'.format(base_url=self.base_url, endpoint=endpoint), headers=self.get_headers()
            )
        if method == 'POST':
            if payload:
                return requests.post(
                    '{base_url}{endpoint}'.format(base_url=self.base_url, endpoint=endpoint),
                    data=payload,
                    headers=self.get_headers(),
                )
            else:
                return requests.post(
                    '{base_url}{endpoint}'.format(base_url=self.base_url, endpoint=endpoint), headers=self.get_headers()
                )

    def get_token(self):
        """
        Get installation token for a Github App, which then allows to access
        the Github API with the permissions of this App.
        """
        endpoint = '/app/installations/{}/access_tokens'.format(self.installation_id)
        for _ in range(NB_RETRIES):
            r = self.make_request(endpoint, method='POST')
            if r.status_code != 200 and r.status_code != 201:
                logger.warning(
                    """Error in HTTP Request for access token.
                Status code: {} Response Text: {}""".format(
                        r.status_code, r.text
                    )
                )
                time.sleep(1)
                continue
            return r.json().get("token")
        raise GithubException(
            """Unable to retrieve an access token.
        Status code: {} Response Text: {}""".format(
                r.status_code, r.text
            )
        )

    def get_installation(self):
        """
        List the installations of the Github App identified by self.app_id,
        and return the first one.

        HACK: we should check the entire list and take the "correct" installation
        for our usage, but as of now we expect the App to only have one installation.
        """
        endpoint = '/app/installations'
        for _ in range(NB_RETRIES):
            r = self.make_request(endpoint, method='GET')
            if r.status_code != 200 and r.status_code != 201:
                logger.warning(
                    """Error in HTTP Request for installation id.
                Status code: {} Response Text: {}""".format(
                        r.status_code, r.text
                    )
                )
                time.sleep(1)
                continue
            return r.json()[0]["id"]
        raise GithubException(
            """Unable to retrieve installation id.
        Status code: {} Response Text: {}""".format(
                r.status_code, r.text
            )
        )
