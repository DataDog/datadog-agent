import http.client
import os
import random
import time
from pathlib import Path

from invoke import task

SERVICE_ACCOUNT_FILE = Path("service-account.json")
SCOPE = ["https://www.googleapis.com/auth/spreadsheets"]


def _execute_with_retries(request, retries: int = 4, min_wait: float = 1.0, max_wait: float = 30.0):
    """
    Execute a googleapiclient HttpRequest with exponential backoff and jitter.

    This function attempts to execute the given Google API request, automatically
    retrying on transient HTTP or network errors. Retries use exponential backoff
    with random jitter to avoid thundering-herd effects. The delay between retries
    grows exponentially until capped by ``max_wait``.

    A request is retried when:
      * It raises ``googleapiclient.errors.HttpError`` with a status code in
        {408, 429, 500, 502, 503, 504}, or
      * It fails due to a transient network error such as ``ConnectionError``,
        ``TimeoutError``, or ``http.client.RemoteDisconnected``.

    Args:
        request (googleapiclient.http.HttpRequest):
            The prepared request object to execute.
        retries (int, optional):
            Maximum number of retry attempts after the initial request.
            Defaults to 4 (for a total of 5 tries).
        min_wait (float, optional):
            Initial backoff delay in seconds before the first retry. Defaults to 1.0.
        max_wait (float, optional):
            Maximum backoff delay (hard cap) in seconds. Defaults to 30.0.

    Raises:
        googleapiclient.errors.HttpError:
            If a non-retryable HTTP error is encountered or retries are exhausted.
        ConnectionError | TimeoutError | http.client.RemoteDisconnected:
            If a transient network error persists after all retries.
        RuntimeError:
            If the retry loop exits unexpectedly (should not normally occur).

    Returns:
        Any: The result of ``request.execute()`` if successful.
    """
    from googleapiclient import errors

    retryable_status = {408, 429, 500, 502, 503, 504}

    for attempt in range(retries + 1):
        try:
            return request.execute()
        except (errors.HttpError, http.client.RemoteDisconnected, TimeoutError, ConnectionError) as e:
            status = getattr(getattr(e, "resp", None), "status", None)
            is_http = isinstance(e, errors.HttpError)

            retryable = (not is_http) or (status in retryable_status)
            if not retryable or attempt == retries:
                raise

            sleep = min_wait * (2**attempt)
            jitter = random.uniform(sleep * 0.5, sleep * 1.5)
            sleep = min(jitter, max_wait)
            print(
                f"[Retry {attempt+1}/{retries}] HTTP {status or 'network'} — "
                f"waiting {sleep:.1f}s before next attempt..."
            )
            time.sleep(sleep)

    # Unreachable, here for sanity. We should either return successfully or raise last error
    raise RuntimeError("Exhausted retries")


@task
def register_deployment_to_sheet(
    ctx,
    pipeline_id=None,
    img_variables=None,
    img_registries=None,
    img_sources=None,
    img_destinations=None,
    img_signing=None,
    apps=None,
    bazel_target=None,
    ddr=None,
    ddr_workflow_id=None,
    target_env=None,
    dynamic_build_render=None,
    spreadsheet_id=None,
):
    """
    Append metadata about a Datadog Agent deployment to a Google Sheet.

    This task collects deployment context (e.g., image variables, Bazel target,
    DDR workflow ID, and pipeline link) from arguments or environment variables,
    and appends a new row to the specified Google Sheets document.

    The function uses a service account credential to authenticate against the
    Google Sheets API, and performs the update with automatic retry handling for
    transient HTTP and network errors (see `_execute_with_retries`).

    Args:
        ctx (invoke.Context): Invoke execution context (unused).
        pipeline_id (str, optional): GitLab pipeline ID. Defaults to $CI_PIPELINE_ID.
        img_variables (str, optional): Image variables. Defaults to $IMG_VARIABLES.
        img_registries (str, optional): Image registries. Defaults to $IMG_REGISTRIES.
        img_sources (str, optional): Image sources. Defaults to $IMG_SOURCES.
        img_destinations (str, optional): Image destinations. Defaults to $IMG_DESTINATIONS.
        img_signing (str, optional): Image signing metadata. Defaults to $IMG_SIGNING.
        apps (str, optional): Application identifiers. Defaults to $APPS.
        bazel_target (str, optional): Bazel build target. Defaults to $BAZEL_TARGET.
        ddr (str, optional): Datadog release reference. Defaults to $DDR.
        ddr_workflow_id (str, optional): DDR workflow identifier. Defaults to $DDR_WORKFLOW_ID.
        target_env (str, optional): Deployment environment. Defaults to $TARGET_ENV.
        dynamic_build_render (str, optional): Dynamic build render parameters.
            Defaults to $DYNAMIC_BUILD_RENDER_TARGET_FORWARD_PARAMETERS.
        spreadsheet_id (str, optional): Target spreadsheet ID. Defaults to $SHEET_ID.

    Raises:
        ValueError: If no spreadsheet ID is provided.
        FileNotFoundError: If the service account credentials file is missing.
        googleapiclient.errors.HttpError: If the Sheets API call fails after retries.

    Returns:
        None: Prints the API response to stdout on success.
    """
    from googleapiclient import discovery
    from oauth2client.service_account import ServiceAccountCredentials

    pipeline_id = pipeline_id or os.environ.get("CI_PIPELINE_ID")
    img_variables = img_variables or os.environ.get("IMG_VARIABLES")
    img_registries = img_registries or os.environ.get("IMG_REGISTRIES")
    img_sources = img_sources or os.environ.get("IMG_SOURCES")
    img_destinations = img_destinations or os.environ.get("IMG_DESTINATIONS")
    img_signing = img_signing or os.environ.get("IMG_SIGNING")
    apps = apps or os.environ.get("APPS")
    bazel_target = bazel_target or os.environ.get("BAZEL_TARGET")
    ddr = ddr or os.environ.get("DDR")
    ddr_workflow_id = ddr_workflow_id or os.environ.get("DDR_WORKFLOW_ID")
    target_env = target_env or os.environ.get("TARGET_ENV")
    dynamic_build_render = dynamic_build_render or os.environ.get("DYNAMIC_BUILD_RENDER_TARGET_FORWARD_PARAMETERS")
    spreadsheet_id = spreadsheet_id or os.environ.get("SHEET_ID")

    if not spreadsheet_id:
        raise ValueError("Missing spreadsheet_id (or SHEET_ID env var)")

    if not SERVICE_ACCOUNT_FILE.exists():
        raise FileNotFoundError(f"Service account file not found: {SERVICE_ACCOUNT_FILE}")

    credentials = ServiceAccountCredentials.from_json_keyfile_name(str(SERVICE_ACCOUNT_FILE), SCOPE)
    service = discovery.build("sheets", "v4", credentials=credentials)
    body = {
        "values": [
            [
                str(img_variables),
                str(img_registries),
                str(img_sources),
                str(img_destinations),
                str(img_signing),
                str(apps),
                str(bazel_target),
                str(ddr),
                str(ddr_workflow_id),
                str(target_env),
                str(dynamic_build_render),
                f"https://gitlab.ddbuild.io/DataDog/datadog-agent/pipelines/{pipeline_id}",
            ]
        ]
    }
    request = (
        service.spreadsheets()
        .values()
        .append(
            spreadsheetId=spreadsheet_id,
            valueInputOption="USER_ENTERED",
            range="Sheet3!A2:L2",
            body=body,
        )
    )

    response = _execute_with_retries(request)
    print(response)
