// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use thiserror::Error;

#[derive(Error, Debug)]
pub enum Error {
    #[error("could not parse socket info: {context}")]
    SocketParsingError { context: String },
}
