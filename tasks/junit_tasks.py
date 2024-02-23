from invoke import task

from tasks.libs.junit_upload_core import junit_upload_from_tgz, repack_macos_junit_tar


@task()
def junit_upload(_, tgz_path):
    """
    Uploads JUnit XML files from an archive produced by the `test` task.
    """

    junit_upload_from_tgz(tgz_path)


@task
def junit_macos_repack(_, infile, outfile):
    """
    Repacks JUnit tgz file from macOS Github Action run, so it would
    contain correct job name and job URL.
    """
    repack_macos_junit_tar(infile, outfile)
