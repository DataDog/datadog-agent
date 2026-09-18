// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

/// Keep in sync with MSI `ConfigureUserCustomActions.AgentPasswordPrivateDataKey`
/// and fleet `agentPasswordPrivateDataKey`.
pub(crate) const INSTALLER_AGENT_PASSWORD_LSA_KEY: &str = "L$datadog_ddagentuser_password";

#[cfg(not(test))]
use std::ptr;

#[cfg(not(test))]
use anyhow::{Result, bail};
#[cfg(not(test))]
use windows_sys::Win32::Security::Authentication::Identity::{
    LSA_HANDLE, LSA_OBJECT_ATTRIBUTES, LSA_UNICODE_STRING, LsaClose, LsaFreeMemory, LsaOpenPolicy,
    LsaRetrievePrivateData, POLICY_GET_PRIVATE_INFORMATION,
};

#[cfg(not(test))]
use super::secure_utf16::{SecureUtf16String, secure_zero_bytes};

#[cfg(not(test))]
const STATUS_OBJECT_NAME_NOT_FOUND: i32 = 0xC000_0034u32 as i32;

#[cfg(not(test))]
pub(crate) fn read_installer_agent_password() -> Result<Option<SecureUtf16String>> {
    let mut key_w = super::wide::null_terminated(INSTALLER_AGENT_PASSWORD_LSA_KEY);
    let key_name = lsa_unicode_string(&mut key_w);

    let object_attributes: LSA_OBJECT_ATTRIBUTES = unsafe { std::mem::zeroed() };
    let mut policy_handle: LSA_HANDLE = 0;
    let status = unsafe {
        LsaOpenPolicy(
            ptr::null(),
            &object_attributes,
            POLICY_GET_PRIVATE_INFORMATION as u32,
            &mut policy_handle,
        )
    };
    if status != 0 {
        bail!("LsaOpenPolicy: NTSTATUS {status:#010x}");
    }
    let policy = PolicyHandle {
        handle: policy_handle,
    };

    let mut secret = ptr::null_mut();
    let status = unsafe { LsaRetrievePrivateData(policy.handle, &key_name, &mut secret) };
    if status == STATUS_OBJECT_NAME_NOT_FOUND {
        return Ok(None);
    }
    if status != 0 {
        bail!(
            "LsaRetrievePrivateData({INSTALLER_AGENT_PASSWORD_LSA_KEY}): NTSTATUS {status:#010x}"
        );
    }

    Ok(LsaSecret { data: secret }.into_password())
}

#[cfg(not(test))]
fn lsa_unicode_string(wide: &mut [u16]) -> LSA_UNICODE_STRING {
    let char_count = wide.len().saturating_sub(1);
    LSA_UNICODE_STRING {
        Length: (char_count * 2) as u16,
        MaximumLength: (wide.len() * 2) as u16,
        Buffer: wide.as_mut_ptr(),
    }
}

#[cfg(not(test))]
struct LsaSecret {
    data: *mut LSA_UNICODE_STRING,
}

#[cfg(not(test))]
impl LsaSecret {
    fn into_password(self) -> Option<SecureUtf16String> {
        if self.data.is_null() {
            return None;
        }
        unsafe {
            let secret = &*self.data;
            if secret.Buffer.is_null() || secret.Length == 0 {
                return None;
            }
            let char_count = secret.Length as usize / 2;
            let slice = std::slice::from_raw_parts(secret.Buffer, char_count);
            Some(SecureUtf16String::from_utf16_units(slice))
        }
    }
}

#[cfg(not(test))]
impl Drop for LsaSecret {
    fn drop(&mut self) {
        if self.data.is_null() {
            return;
        }
        unsafe {
            let secret = &*self.data;
            if !secret.Buffer.is_null() && secret.Length > 0 {
                secure_zero_bytes(std::slice::from_raw_parts_mut(
                    secret.Buffer.cast(),
                    secret.Length as usize,
                ));
            }
            LsaFreeMemory(self.data.cast());
            self.data = ptr::null_mut();
        }
    }
}

#[cfg(not(test))]
struct PolicyHandle {
    handle: LSA_HANDLE,
}

#[cfg(not(test))]
impl Drop for PolicyHandle {
    fn drop(&mut self) {
        if self.handle != 0 {
            unsafe {
                LsaClose(self.handle);
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const MSI_CS: &str = include_str!(
        "../../../../../../tools/windows/DatadogAgentInstaller/CustomActions/ConfigureUserCustomActions.cs"
    );
    const FLEET_GO: &str =
        include_str!("../../../../../../pkg/fleet/installer/packages/user/windows/user.go");

    #[test]
    fn installer_lsa_key_matches_msi_and_fleet() {
        assert_eq!(
            csharp_agent_password_key(MSI_CS),
            INSTALLER_AGENT_PASSWORD_LSA_KEY,
            "drift from MSI ConfigureUserCustomActions.AgentPasswordPrivateDataKey"
        );
        assert_eq!(
            go_agent_password_key(FLEET_GO),
            INSTALLER_AGENT_PASSWORD_LSA_KEY,
            "drift from fleet agentPasswordPrivateDataKey"
        );
    }

    #[test]
    fn csharp_key_parser_reads_interpolated_lsa_name() {
        let src = r#"
            public static string AgentPasswordPrivateDataKey()
            {
                var secretType = "L$";
                return $"{secretType}datadog_ddagentuser_password";
            }
        "#;
        assert_eq!(
            csharp_agent_password_key(src),
            "L$datadog_ddagentuser_password"
        );
    }

    #[test]
    fn go_key_parser_reads_returned_literal() {
        let src = r#"
            func agentPasswordPrivateDataKey() string {
                return "L$datadog_ddagentuser_password"
            }
        "#;
        assert_eq!(go_agent_password_key(src), "L$datadog_ddagentuser_password");
    }

    fn csharp_agent_password_key(src: &str) -> String {
        let body = function_body(src, "AgentPasswordPrivateDataKey()");
        if let Some(literal) = returned_quoted_string(body) {
            return literal.to_string();
        }
        let prefix = assigned_quoted_string(body, "secretType")
            .expect("MSI AgentPasswordPrivateDataKey must assign secretType");
        let suffix = interpolated_return_suffix(body, "secretType").expect(
            "MSI AgentPasswordPrivateDataKey must return $\"{secretType}...\" or a string literal",
        );
        format!("{prefix}{suffix}")
    }

    fn go_agent_password_key(src: &str) -> String {
        let body = function_body(src, "func agentPasswordPrivateDataKey()");
        returned_quoted_string(body)
            .expect("fleet agentPasswordPrivateDataKey must return a string literal")
            .to_string()
    }

    fn function_body<'a>(src: &'a str, signature: &str) -> &'a str {
        let start = src
            .find(signature)
            .unwrap_or_else(|| panic!("missing {signature}"));
        let after = &src[start + signature.len()..];
        let open = after
            .find('{')
            .unwrap_or_else(|| panic!("missing '{{' after {signature}"));
        brace_inner(&after[open..])
    }

    fn brace_inner(src: &str) -> &str {
        let mut depth = 0;
        for (i, b) in src.bytes().enumerate() {
            match b {
                b'{' => depth += 1,
                b'}' => {
                    depth -= 1;
                    if depth == 0 {
                        return &src[1..i];
                    }
                }
                _ => {}
            }
        }
        panic!("unbalanced braces");
    }

    fn returned_quoted_string(body: &str) -> Option<&str> {
        let marker = "return \"";
        let start = body.find(marker)? + marker.len();
        let end = body[start..].find('"')?;
        Some(&body[start..start + end])
    }

    fn assigned_quoted_string<'a>(body: &'a str, var: &str) -> Option<&'a str> {
        let marker = format!("{var} = \"");
        let start = body.find(&marker)? + marker.len();
        let end = body[start..].find('"')?;
        Some(&body[start..start + end])
    }

    fn interpolated_return_suffix<'a>(body: &'a str, var: &str) -> Option<&'a str> {
        let marker = format!("return $\"{{{var}}}");
        let start = body.find(&marker)? + marker.len();
        let end = body[start..].find('"')?;
        Some(&body[start..start + end])
    }
}
