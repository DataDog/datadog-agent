"""Tokenless GitLab task client for the repository-scoped Bits Code proxy."""

import os
from urllib.parse import quote, urlsplit

import gitlab
import requests


class BitsCodeGitlab(gitlab.Gitlab):
    def __init__(self):
        host = os.environ.get("GITLAB_HOST") or "https://gitlab.ddbuild.io"
        if "://" not in host:
            host = f"https://{host}"
        endpoint = urlsplit(host)
        if (
            endpoint.scheme != "https"
            or not endpoint.hostname
            or endpoint.username is not None
            or endpoint.password is not None
            or endpoint.path not in ("", "/")
            or endpoint.query
            or endpoint.fragment
        ):
            raise ValueError("GITLAB_HOST must be an HTTPS host, without credentials, a path, query or fragment")

        session = requests.Session()
        # Do not pick up workstation .netrc credentials or a second HTTP proxy.
        session.trust_env = False
        session.max_redirects = 0
        super().__init__(
            host,
            session=session,
            ssl_verify=os.environ.get("REQUESTS_CA_BUNDLE") or os.environ.get("SSL_CERT_FILE") or True,
            timeout=30,
            retry_transient_errors=True,
            keep_base_url=True,
        )
        self._project_paths = {}

    def register_project(self, project_id, project_path):
        # Keep the SDK object's numeric ID intact; only its transport URL changes.
        self._project_paths[str(project_id)] = quote(project_path, safe="")

    def _build_url(self, path):
        url = urlsplit(super()._build_url(path))
        base = urlsplit(self.url)
        if (url.scheme, url.netloc) != (base.scheme, base.netloc):
            raise ValueError("GitLab API requests must use the configured Bits Code endpoint")
        parts = url.path.split("/")
        if parts[:4] == ["", "api", "v4", "projects"] and len(parts) >= 5:
            parts[4] = self._project_paths.get(parts[4], parts[4])
            url = url._replace(path="/".join(parts))
        return url.geturl()
