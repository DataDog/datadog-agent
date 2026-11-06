from invoke import task

from tasks.libs.common.junit_upload_core import junit_upload_from_tgz


@task()
def junit_upload(_, tgz_path, result_json, max_workers=None, report_timings=False):
    """
    Uploads JUnit XML files from an archive produced by the `test` task.

    max_workers optionally limits concurrent uploader processes. report_timings
    emits per-upload queue/run durations without arguments or environment data.
    """

    junit_upload_from_tgz(
        tgz_path,
        result_json,
        max_workers=None if max_workers is None else int(max_workers),
        report_timings=report_timings,
    )
