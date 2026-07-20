"""SAP Cloud ALM custom Agent check.

Polls the SAP Cloud ALM Analytics API and submits, for each configured provider
query, the most recent value of every returned measure as a gauge. Config-driven:
each provider entry declares its own response format, resolution, semantic window,
dimensions, and measures.

Authentication (OAuth2-ready):
  * If the instance declares an ``oauth`` block (token_url, client_id,
    client_secret), the check fetches an OAuth2 client-credentials token, caches
    it (with expiry) on the check instance, refreshes it when expired, and sends
    ``Authorization: Bearer <token>``. This is how a real SAP Cloud ALM tenant is
    reached.
  * Else if ``api_key`` is set, the check sends the ``APIKey`` header. This is how
    the public SAP Business Accelerator Hub sandbox is reached.
  * Else the check logs a config error and reports CRITICAL.

Verified live contract:
  POST {sandbox_url}{api_base_path}/analytics/providers/data
    sandbox : sandbox_url=https://sandbox.api.sap.com/SAPCALM
              api_base_path=/calm-analytics/v1          (APIKey header)
    tenant  : sandbox_url=https://<tenant>.<region>.alm.cloud.sap
              api_base_path=/api/calm-analytics/v1      (OAuth2 Bearer)
  headers: auth (APIKey|Bearer), Content-Type/Accept: application/json
  body:
    {"format":<fmt>,"timestampFormat":"unix",
     "timeRange":{"semantic":<sem>},"resolution":<res>,
     "queries":[{"name":"q","provider":<name>,
                 "columns":{"dimensions":[...],
                            "metrics":[{"measure":..,"method":..}, ...]}}]}
    * field is ``provider`` (NOT dataProvider).
    * resolution single-letter: H/D/W/M/Y.
    * semantic relative window: L1D/L7D/L30D.
    * method: AVG/SUM/MAX/MIN/LAST.
    * no measures -> "metrics":[] (provider returns its default measure).

  Response shape depends on ``format``:
    "time_series" -> DOUBLE-nested:
        [[ {serieName, measure, attributes:[{key,value},...],
            dataPoints:[[value,tsMillis],...]}, ... ]]
      resp[0] is the list of series. Each serie is tagged from its
      ``attributes`` (key=dimension name, value=dim value). The metric value is
      the value of the dataPoint with the MAX timestamp.
    "table" -> SINGLE-nested:
        [ {columns:[{text,type},...], rows:[[...],...],
           dimensions:[...], measures:[...]}, ... ]
      Columns of type "string" are dimension columns (tag key=text,
      value=row cell); columns of type "number" are measure columns (metric name
      from text, value=row cell). One gauge is emitted per (row x measure column).

Verified working sandbox providers (only these two resolve on the demo tenant):
  * EXM_DATAPROVIDER: default measure ``counter``; dimension ``useCase`` (e.g. "IM").
    format time_series, resolution H, semantic L1D, dimensions ["useCase"].
  * DEMO_TASKS: measure ``count`` (method SUM); dimensions ``status`` and
    ``project`` (queried separately). format table, resolution D, semantic L30D.

Emitted metrics: ``sap.alm.<measure>`` (lowercased, non-alphanumerics -> "_"),
e.g. sap.alm.counter, sap.alm.count. Tags: always ``provider:<name>``, plus one
``<dimkey_lowercased>:<value>`` tag per dimension.

To switch to a real tenant: set sandbox_url to the tenant base URL, set
api_base_path to ``/api/calm-analytics/v1``, and supply an ``oauth`` block
instead of api_key.
"""

import re
import time

from datadog_checks.base import AgentCheck

try:
    import requests
except ImportError:
    requests = None

DEFAULT_SANDBOX_URL = "https://sandbox.api.sap.com/SAPCALM"
DEFAULT_API_BASE_PATH = "/calm-analytics/v1"
DATA_PATH = "/analytics/providers/data"

SERVICE_CHECK = "sap.alm.can_connect"

HTTP_TIMEOUT = 20
# Refresh the OAuth2 token this many seconds before it actually expires.
TOKEN_EXPIRY_SKEW = 30

_METRIC_SANITIZE = re.compile(r"[^0-9a-z]+")


def _sanitize_measure(measure):
    """Lowercase and replace runs of non-alphanumerics with a single underscore."""
    return _METRIC_SANITIZE.sub("_", (measure or "").lower()).strip("_")


class SapAlmCheck(AgentCheck):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        # OAuth2 token cache, keyed on the token_url so distinct tenants don't
        # clobber each other across checks.
        self._token = None
        self._token_expiry = 0.0
        self._token_key = None

    def check(self, instance):
        sandbox_url = (instance.get("sandbox_url") or DEFAULT_SANDBOX_URL).rstrip("/")
        api_base_path = instance.get("api_base_path") or DEFAULT_API_BASE_PATH
        providers = instance.get("providers") or []

        base_tags = list(instance.get("tags") or [])
        base_tags.append(f"sandbox_url:{sandbox_url}")

        if requests is None:
            self.log.error("sap_alm: 'requests' library unavailable")
            self.service_check(
                SERVICE_CHECK,
                AgentCheck.CRITICAL,
                tags=base_tags,
                message="requests unavailable",
            )
            return

        try:
            auth_headers = self._auth_headers(instance)
        except Exception as exc:  # noqa: BLE001 - surface a clear config error
            self.log.error("sap_alm: authentication failed: %s", exc)
            self.service_check(
                SERVICE_CHECK,
                AgentCheck.CRITICAL,
                tags=base_tags,
                message=f"authentication failed: {exc}",
            )
            return

        if auth_headers is None:
            self.log.error(
                "sap_alm: no credentials configured; set an 'oauth' block "
                "(token_url/client_id/client_secret) or 'api_key'"
            )
            self.service_check(
                SERVICE_CHECK,
                AgentCheck.CRITICAL,
                tags=base_tags,
                message="missing credentials (oauth block or api_key)",
            )
            return

        url = sandbox_url + api_base_path + DATA_PATH
        headers = {"Content-Type": "application/json", "Accept": "application/json"}
        headers.update(auth_headers)

        overall_ok = True
        any_provider = False

        for provider in providers:
            any_provider = True
            name = provider.get("name")
            if not name:
                self.log.warning("sap_alm: provider entry without 'name'; skipping")
                continue

            fmt = provider.get("format", "time_series")
            resolution = provider.get("resolution", "H")
            semantic = provider.get("semantic", "L1D")
            dimensions = provider.get("dimensions") or []
            measures = provider.get("measures") or []

            ptags = base_tags + [f"provider:{name}"]

            body = {
                "format": fmt,
                "timestampFormat": "unix",
                "timeRange": {"semantic": semantic},
                "resolution": resolution,
                "queries": [
                    {
                        "name": "q",
                        "provider": name,
                        "columns": {
                            "dimensions": list(dimensions),
                            "metrics": list(measures),
                        },
                    }
                ],
            }

            try:
                resp = requests.post(url, json=body, headers=headers, timeout=HTTP_TIMEOUT)
            except Exception as exc:  # noqa: BLE001 - continue to next provider
                overall_ok = False
                self.log.error("sap_alm: request failed for provider %s: %s", name, exc)
                continue

            if resp.status_code != 200:
                overall_ok = False
                self.log.error(
                    "sap_alm: provider %s returned HTTP %s: %s",
                    name,
                    resp.status_code,
                    resp.text[:200],
                )
                continue

            try:
                payload = resp.json()
            except ValueError as exc:
                overall_ok = False
                self.log.error(
                    "sap_alm: provider %s non-JSON response: %s: %s",
                    name,
                    exc,
                    resp.text[:200],
                )
                continue

            try:
                if fmt == "table":
                    self._submit_table(payload, name, ptags)
                else:
                    self._submit_time_series(payload, name, ptags)
            except Exception as exc:  # noqa: BLE001 - one bad payload must not kill the rest
                overall_ok = False
                self.log.error(
                    "sap_alm: failed to parse %s payload for provider %s: %s",
                    fmt,
                    name,
                    exc,
                )
                continue

        status = AgentCheck.OK if (overall_ok and any_provider) else AgentCheck.CRITICAL
        self.service_check(SERVICE_CHECK, status, tags=base_tags)

    # ------------------------------------------------------------------ auth

    def _auth_headers(self, instance):
        """Return the auth headers dict, or None when no credentials are set."""
        oauth = instance.get("oauth")
        if oauth:
            token = self._oauth_token(oauth)
            return {"Authorization": f"Bearer {token}"}

        api_key = instance.get("api_key")
        if api_key:
            return {"APIKey": api_key}

        return None

    def _oauth_token(self, oauth):
        """Fetch/cache an OAuth2 client-credentials access token."""
        token_url = oauth.get("token_url")
        client_id = oauth.get("client_id")
        client_secret = oauth.get("client_secret")
        if not (token_url and client_id and client_secret):
            raise ValueError("oauth block requires token_url, client_id, client_secret")

        now = time.time()
        if self._token and self._token_key == token_url and now < self._token_expiry - TOKEN_EXPIRY_SKEW:
            return self._token

        resp = requests.post(
            token_url,
            data={"grant_type": "client_credentials"},
            auth=(client_id, client_secret),
            headers={"Accept": "application/json"},
            timeout=HTTP_TIMEOUT,
        )
        if resp.status_code != 200:
            raise ValueError(f"token endpoint returned HTTP {resp.status_code}: {resp.text[:200]}")
        data = resp.json()
        access_token = data.get("access_token")
        if not access_token:
            raise ValueError("token response missing access_token")

        expires_in = data.get("expires_in")
        try:
            expires_in = float(expires_in)
        except (TypeError, ValueError):
            expires_in = 3600.0

        self._token = access_token
        self._token_expiry = now + expires_in
        self._token_key = token_url
        return access_token

    # -------------------------------------------------------------- parsing

    def _submit_time_series(self, payload, provider, ptags):
        """Parse the DOUBLE-nested time_series response and emit one gauge per serie."""
        outer = self._as_list(payload)
        series = self._as_list(outer[0]) if outer else []
        for serie in series:
            if not isinstance(serie, dict):
                continue
            measure = serie.get("measure") or ""
            metric = "sap.alm.{}".format(_sanitize_measure(measure) or "value")

            tags = list(ptags)
            for attr in self._as_list(serie.get("attributes")):
                if not isinstance(attr, dict):
                    continue
                key = attr.get("key")
                value = attr.get("value")
                if key is None or value is None:
                    continue
                tags.append(f"{str(key).lower()}:{value}")

            latest = self._latest_value(serie.get("dataPoints") or [])
            if latest is None:
                self.log.debug(
                    "sap_alm: provider %s measure %s has no dataPoints",
                    provider,
                    measure,
                )
                continue
            self.gauge(metric, latest, tags=tags)

    def _submit_table(self, payload, provider, ptags):
        """Parse the SINGLE-nested table response; one gauge per (row x measure)."""
        for result in self._as_list(payload):
            if not isinstance(result, dict):
                continue
            columns = self._as_list(result.get("columns"))
            rows = self._as_list(result.get("rows"))

            dim_indexes = []  # [(col_index, tag_key)]
            measure_indexes = []  # [(col_index, metric_name)]
            for idx, col in enumerate(columns):
                if not isinstance(col, dict):
                    continue
                col_type = (col.get("type") or "").lower()
                text = col.get("text") or ""
                if col_type == "string":
                    dim_indexes.append((idx, str(text).lower()))
                elif col_type == "number":
                    measure_indexes.append((idx, "sap.alm.{}".format(_sanitize_measure(text) or "value")))

            for row in rows:
                if not isinstance(row, (list, tuple)):
                    continue
                row_tags = list(ptags)
                for idx, tag_key in dim_indexes:
                    if idx < len(row) and row[idx] is not None:
                        row_tags.append(f"{tag_key}:{row[idx]}")
                for idx, metric in measure_indexes:
                    if idx >= len(row):
                        continue
                    try:
                        value = float(row[idx])
                    except (TypeError, ValueError):
                        continue
                    self.gauge(metric, value, tags=row_tags)

    @staticmethod
    def _as_list(value):
        return value if isinstance(value, list) else []

    @staticmethod
    def _latest_value(data_points):
        """Return the value of the dataPoint with the max timestamp.

        Each dataPoint is [value, timestampMillis]. Returns None if none valid.
        """
        best_ts = None
        best_val = None
        for dp in data_points:
            if not isinstance(dp, (list, tuple)) or len(dp) < 2:
                continue
            value, ts = dp[0], dp[1]
            try:
                value = float(value)
                ts = float(ts)
            except (TypeError, ValueError):
                continue
            if best_ts is None or ts > best_ts:
                best_ts = ts
                best_val = value
        return best_val
