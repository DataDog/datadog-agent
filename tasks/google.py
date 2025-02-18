import os

from invoke import task


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
    from googleapiclient import discovery
    from oauth2client.service_account import ServiceAccountCredentials

    pipeline_id = (
        os.environ.get("CI_PIPELINE_ID") if not pipeline_id else pipeline_id if not pipeline_id else pipeline_id
    )
    img_variables = os.environ.get("IMG_VARIABLES") if not img_variables else img_variables
    img_registries = os.environ.get("IMG_REGISTRIES") if not img_registries else img_registries
    img_sources = os.environ.get("IMG_SOURCES") if not img_sources else img_sources
    img_destinations = os.environ.get("IMG_DESTINATIONS") if not img_destinations else img_destinations
    img_signing = os.environ.get("IMG_SIGNING") if not img_signing else img_signing
    apps = os.environ.get("APPS") if not apps else apps
    bazel_target = os.environ.get("BAZEL_TARGET") if not bazel_target else bazel_target
    ddr = os.environ.get("DDR") if not ddr else ddr
    ddr_workflow_id = os.environ.get("DDR_WORKFLOW_ID") if not ddr_workflow_id else ddr_workflow_id
    target_env = os.environ.get("TARGET_ENV") if not target_env else target_env
    dynamic_build_render = (
        os.environ.get("DYNAMIC_BUILD_RENDER_TARGET_FORWARD_PARAMETERS")
        if not dynamic_build_render
        else dynamic_build_render
    )
    spreadsheet_id = os.environ.get("SHEET_ID") if not spreadsheet_id else spreadsheet_id

    scope = ["https://www.googleapis.com/auth/spreadsheets"]
    credentials = ServiceAccountCredentials.from_json_keyfile_name("service-account.json", scope)

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
                "https://gitlab.ddbuild.io/DataDog/datadog-agent/pipelines/" + pipeline_id,
            ]
        ]
    }

    request = (
        service.spreadsheets()
        .values()
        .append(
            spreadsheetId=spreadsheet_id,
            valueInputOption="USER_ENTERED",
            range="Sheet1!A1:L1",
            body=body,
        )
    )
    response = request.execute()

    print(response)
