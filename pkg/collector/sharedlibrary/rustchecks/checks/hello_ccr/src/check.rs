use anyhow::Result;

use core::*;

/// Hostname of the process running this check.
///
/// On a Cluster Check Runner this resolves to the CCR pod name (Kubernetes sets
/// the pod's UTS hostname and the HOSTNAME env var to the pod name), which is
/// exactly what proves *where* the dispatched check was scheduled.
fn runner_hostname() -> String {
    std::env::var("HOSTNAME")
        .ok()
        .filter(|h| !h.is_empty())
        .or_else(|| {
            std::fs::read_to_string("/etc/hostname")
                .ok()
                .map(|s| s.trim().to_owned())
                .filter(|s| !s.is_empty())
        })
        .unwrap_or_else(|| "unknown".to_owned())
}

/// Emits a heartbeat tagged with the runner pod so we can see, in Datadog, which
/// CCR (or node agent) actually ran the check.
pub fn check(check: &AgentCheck) -> Result<()> {
    let host = runner_hostname();
    let tags = vec![
        format!("runner_pod:{host}"),
        "poc:rust-cluster-check".to_owned(),
    ];

    // Empty hostname ("") is deliberate: the cluster-check dispatcher patches
    // `empty_default_hostname: true` onto dispatched instances, so we carry the
    // runner identity as the `runner_pod` tag rather than host-scoping the point.
    check.gauge("hello.ccr.heartbeat", 1.0, &tags, "", false)?;

    check.service_check(
        "hello.ccr.status",
        ServiceCheckStatus::OK,
        &tags,
        "",
        &format!("rust shared-library check scheduled on {host}"),
    )?;

    Ok(())
}
