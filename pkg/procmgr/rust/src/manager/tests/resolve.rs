// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use super::super::*;
use super::{loader, sleep_def, test_runtime_context};
use crate::uuid_gen::{SequentialUuidGenerator, UuidGenerator};
use std::sync::Arc;
use tonic::Status;

#[tokio::test]
async fn test_ambiguous_uuid_prefix_returns_error() {
    let uuid_gen: Arc<dyn UuidGenerator> = Arc::new(SequentialUuidGenerator::new(vec![
        "aabbccdd-1111-0000-0000-000000000000",
        "aabbccdd-2222-0000-0000-000000000000",
    ]));
    let mgr = ProcessManager::new(
        loader(vec![sleep_def("svc-a"), sleep_def("svc-b")]),
        uuid_gen,
    );
    let (handles, _rx) = test_runtime_context();

    let err: Status = mgr.handle_start("aabbccdd", &handles).await.unwrap_err();
    assert_eq!(err.code(), tonic::Code::InvalidArgument);
    assert!(
        err.message().contains("ambiguous"),
        "error should mention ambiguity: {}",
        err.message()
    );

    mgr.handle_start("aabbccdd-1", &handles)
        .await
        .expect("unambiguous prefix should resolve");

    let _: Result<_, _> = mgr.handle_stop("aabbccdd-1").await;
}
