"""Review SMP experiment health reports without uploading them."""

from invoke import task

from tasks.libs.smp.experiment_health import read_json, render_artifacts, summarize, write_json


@task
def experiment_health(
    _, smp='./smp', report='outputs/report.v1.json', output='outputs/experiment-health', pr_context='pr_context.json'
):
    """Render container experiment health artifacts for review."""
    result = render_artifacts(smp, report, output, 'test/regression/container', pr_context)
    print(f'Local SMP health report: {result}. Upload disabled.')


@task(iterable=['input'])
def summarize_health(_, input, output='experiment-health-summary.json', experiment=None):
    """Compare all evaluations with the latest reported run per PR and experiment."""
    if not input:
        raise ValueError('Supply at least one --input evaluations.json file')
    write_json(output, summarize([read_json(path) for path in input], experiment))
