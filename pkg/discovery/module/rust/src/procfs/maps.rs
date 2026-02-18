// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::fs::File;
use std::io::{BufReader, Read, Take};

use super::root_path;

const MAPS_READ_LIMIT: u64 = 4 * 1024 * 1024 * 1024; // 4GiB

pub fn get_reader_for_pid(pid: i32) -> Result<BufReader<Take<File>>, std::io::Error> {
    let maps_path = root_path().join(pid.to_string()).join("maps");
    let file = File::open(maps_path)?;
    Ok(BufReader::new(file.take(MAPS_READ_LIMIT)))
}
