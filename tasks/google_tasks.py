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
    response = request.execute()

    print(response)
