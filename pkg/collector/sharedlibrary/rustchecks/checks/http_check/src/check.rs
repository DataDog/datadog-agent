use anyhow::{Result, bail};
use std::time::Instant;

use core::*;

/// HTTP check implementation for the core agent's Rust check framework.
///
/// This is a simplified HTTP check that demonstrates the pattern of building
/// checks in the core agent workspace using the `core` crate's `AgentCheck` API.
/// For the full-featured HTTP check, use the ACR's http-check-plugin.
///
/// Submitted data:
///   - Metric: `network.http.response_time` (gauge, seconds)
///   - Metric: `network.http.can_connect` (gauge, 1.0 on success, 0.0 on failure)
///   - Service check: `http.can_connect` (OK on success, CRITICAL on failure)
pub fn check(agent_check: &AgentCheck) -> Result<()> {
    // Parse instance config fields
    let url: String = agent_check.instance.get("url")?;
    let method: String = agent_check
        .instance
        .get("method")
        .unwrap_or_else(|_| "GET".to_string());
    let name: String = agent_check
        .instance
        .get("name")
        .unwrap_or_else(|_| String::new());
    let collect_response_time: bool = agent_check
        .instance
        .get("collect_response_time")
        .unwrap_or(true);

    // Build tags
    let mut tags = Vec::new();
    tags.push(format!("instance:{}", name));
    tags.push(format!("url:{}", url));

    let start = Instant::now();

    // Make the HTTP request
    let result = make_request(&url, &method);
    let elapsed = start.elapsed();

    match result {
        Ok(status) => {
            // Submit response time
            if collect_response_time {
                let response_time_secs = elapsed.as_millis() as f64 / 1000.0;
                agent_check.gauge(
                    "network.http.response_time",
                    response_time_secs,
                    &tags,
                    "",
                    false,
                )?;
            }

            // Check if the status code indicates success (1xx, 2xx, 3xx)
            let success = (100..400).contains(&status);

            let can_connect = if success { 1.0 } else { 0.0 };
            agent_check.gauge("network.http.can_connect", can_connect, &tags, "", false)?;

            let sc_status = if success {
                ServiceCheckStatus::OK
            } else {
                ServiceCheckStatus::CRITICAL
            };
            let message = format!("HTTP {} {}", method, status);
            agent_check.service_check("http.can_connect", sc_status, &tags, "", &message)?;
        }
        Err(e) => {
            // Connection failed
            agent_check.gauge("network.http.can_connect", 0.0, &tags, "", false)?;
            agent_check.service_check(
                "http.can_connect",
                ServiceCheckStatus::CRITICAL,
                &tags,
                "",
                &format!("Connection failed: {}", e),
            )?;
        }
    }

    Ok(())
}

/// Make an HTTP request and return the status code.
fn make_request(url: &str, method: &str) -> Result<u16> {
    let config = ureq::Agent::config_builder()
        .http_status_as_error(false)
        .build();
    let agent = ureq::Agent::new_with_config(config);

    let response = match method.to_uppercase().as_str() {
        "GET" => agent.get(url).call(),
        "POST" => agent.post(url).send_empty(),
        "HEAD" => agent.head(url).call(),
        "PUT" => agent.put(url).send_empty(),
        "DELETE" => agent.delete(url).call(),
        other => bail!("Unsupported HTTP method: {}", other),
    };

    match response {
        Ok(resp) => Ok(resp.status().as_u16()),
        Err(e) => Err(e.into()),
    }
}
