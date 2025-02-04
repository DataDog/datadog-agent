from googleapiclient import discovery
from oauth2client.service_account import ServiceAccountCredentials
import os

from invoke import task

"""
      --variable IMG_VARIABLES
      --variable IMG_REGISTRIES
      --variable IMG_SOURCES
      --variable IMG_DESTINATIONS
      --variable IMG_SIGNING
      --variable APPS
      --variable BAZEL_TARGET
      --variable DDR
      --variable DDR_WORKFLOW_ID
      --variable TARGET_ENV
      --variable DYNAMIC_BUILD_RENDER_TARGET_FORWARD_PARAMETERS"
"""

@task
def register_deployment_to_sheet(ctx):
    pipeline_id = os.environ.get("CI_PIPELINE_ID")
    action = os.environ.get("ACTION", "unknown")
    auto_release = os.environ.get("AUTO_RELEASE", "unknown")
    build_pipeline_id = os.environ.get("BUILD_PIPELINE_ID", "unknown")
    release_product = os.environ.get("RELEASE_PRODUCT", "unknown")
    release_version = os.environ.get("RELEASE_VERSION", "unknown")
    target_repo = os.environ.get("TARGET_REPO", "unknown")
    target_repo_branch = os.environ.get("TARGET_REPO_BRANCH", "unknown")
    spreadsheet_id = os.environ.get("SHEET_ID")

    scope = ["https://www.googleapis.com/auth/spreadsheets"]
    credentials = ServiceAccountCredentials.from_json_keyfile_name(
        "service-account.json", scope
    )

    service = discovery.build("sheets", "v4", credentials=credentials)

    body = {
        "values": [
            [
                str(action),
                str(auto_release),
                str(build_pipeline_id),
                str(release_product),
                str(release_version),
                str(target_repo),
                str(target_repo_branch),
                "https://gitlab.ddbuild.io/DataDog/agent-release-management/pipelines/"
                + pipeline_id,
            ]
        ]
    }

    request = (
        service.spreadsheets()
        .values()
        .append(
            spreadsheetId=spreadsheet_id,
            valueInputOption="USER_ENTERED",
            range="Sheet2!A1:H1",
            body=body,
        )
    )
    response = request.execute()

    print(response)
