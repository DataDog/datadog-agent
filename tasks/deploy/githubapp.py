import base64
import logging
import os
import time

import jwt
import requests

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
            self.installation_id = self.get_installation()

    def gen_payload(self):
        epoch = int(time.time())
        payload = {'iat': epoch, 'exp': epoch + (5 * 60), 'iss': self.app_id}
        return payload

    def get_headers(self):
        payload = self.gen_payload()
        bearer_token = jwt.encode(payload, base64.b64decode(self.key_b64), algorithm='RS256').decode()
        headers = {
            'Authorization': 'Bearer {}'.format(bearer_token),
            'Accept': 'application/vnd.github.machine-man-preview+json',
        }
        return headers

    def make_request(self, endpoint, method='GET', payload=None):
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

    def do_i_have_access(self, owner, repo):
        endpoint = '/repos/{owner}/{repo}/installation'.format(owner=owner, repo=repo)
        r = self.make_request(endpoint)
        return r
