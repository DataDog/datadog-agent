from invoke import task

from tasks.libs.common.junit_upload_core import junit_upload_from_tgz


@task()
def junit_upload(_, tgz_path):
    """
    Uploads JUnit XML files from an archive produced by the `test` task.
    """

    junit_upload_from_tgz(tgz_path)
