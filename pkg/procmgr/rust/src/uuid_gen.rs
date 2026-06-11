// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

pub trait UuidGenerator: Send + Sync {
    fn generate(&self) -> String;
}

pub struct V4UuidGenerator;

impl UuidGenerator for V4UuidGenerator {
    fn generate(&self) -> String {
        uuid::Uuid::new_v4().to_string()
    }
}

#[cfg(test)]
pub struct SequentialUuidGenerator {
    uuids: std::sync::Mutex<Vec<String>>,
}

#[cfg(test)]
impl SequentialUuidGenerator {
    pub fn new(uuids: Vec<&str>) -> Self {
        Self {
            uuids: std::sync::Mutex::new(uuids.into_iter().map(String::from).collect()),
        }
    }
}

#[cfg(test)]
impl UuidGenerator for SequentialUuidGenerator {
    fn generate(&self) -> String {
        self.uuids.lock().unwrap().remove(0)
    }
}
