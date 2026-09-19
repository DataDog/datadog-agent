from invoke import task

from tasks.libs.common.junit_upload_core import junit_upload_from_tgz


@task(help={"collapse_retries": "Report a test retried by gotestsum once, with the result of its deciding attempt"})
def junit_upload(_, tgz_path, result_json, collapse_retries=False):
    """
    Uploads JUnit XML files from an archive produced by the `test` task.
    """

    junit_upload_from_tgz(tgz_path, result_json, collapse_retries=collapse_retries)
