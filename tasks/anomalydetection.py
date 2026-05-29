"""
Invoke tasks for anomaly detection dev tooling (not part of agent build).
"""

from invoke import task


@task
def build_scorer(ctx):
    """
    Builds the anomalydetection-scorer binary to bin/anomalydetection-scorer.
    """
    ctx.run("GOWORK=off go build -C internal/qbranch/anomalydetection-scorer -o ../../../bin/anomalydetection-scorer .")
