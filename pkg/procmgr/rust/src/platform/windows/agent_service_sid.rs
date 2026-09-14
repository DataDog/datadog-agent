// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

use anyhow::Result;

use super::sid::lookup_account_sid;

pub(crate) fn lookup_installed_user_sid(domain: &str, user: &str) -> Result<Vec<u8>> {
    let mut last_err = None;
    for (candidate_domain, candidate_user) in installed_user_lookup_candidates(domain, user) {
        match lookup_account_sid(&candidate_domain, &candidate_user) {
            Ok(sid) => return Ok(sid),
            Err(err) => last_err = Some(err),
        }
    }
    Err(last_err.unwrap_or_else(|| {
        anyhow::anyhow!("no lookup candidates for installed agent user {domain}\\{user}")
    }))
}

fn installed_user_lookup_candidates(domain: &str, user: &str) -> Vec<(String, String)> {
    let mut candidates = vec![(domain.to_string(), user.to_string())];
    if !domain.is_empty() {
        candidates.push((String::new(), format!("{user}@{domain}")));
        candidates.push((String::new(), user.to_string()));
    }
    candidates
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn installed_user_lookup_candidates_include_upn_and_default_domain() {
        let candidates = installed_user_lookup_candidates("datadogqalab.com", "TestUser");
        assert_eq!(
            candidates,
            vec![
                ("datadogqalab.com".to_string(), "TestUser".to_string()),
                (String::new(), "TestUser@datadogqalab.com".to_string()),
                (String::new(), "TestUser".to_string()),
            ]
        );
    }
}
