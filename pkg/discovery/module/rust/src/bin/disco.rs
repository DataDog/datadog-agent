// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use clap::Parser;
use dd_discovery::{Params, get_services};

#[derive(Parser, Debug)]
#[command(name = "disco")]
#[command(about = "Service discovery tool - detects service information from a process PID", long_about = None)]
struct Args {
    /// Process ID to analyze
    #[arg(short, long)]
    pid: i32,
}

#[allow(clippy::print_stdout, clippy::print_stderr)]
fn main() {
    let args = Args::parse();

    // Create params with the single PID
    let params = Params {
        new_pids: Some(vec![args.pid]),
        heartbeat_pids: None,
    };

    // Run service detection
    let response = get_services(params);

    // Output the results as JSON
    match serde_json::to_string_pretty(&response) {
        Ok(json) => println!("{}", json),
        Err(e) => eprintln!("Error serializing response: {}", e),
    }
}
