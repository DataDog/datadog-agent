use core::*;

use std::time::{SystemTime, UNIX_EPOCH};
use rustls::pki_types::CertificateDer;
use x509_parser::parse_x509_certificate;

// references for certificate expiration
const DEFAULT_EXPIRE_DAYS_WARNING: i32 = 14;
const DEFAULT_EXPIRE_DAYS_CRITICAL: i32 = 7;
const DEFAULT_EXPIRE_WARNING: i32 = DEFAULT_EXPIRE_DAYS_WARNING * 24 * 3600;
const DEFAULT_EXPIRE_CRITICAL: i32 = DEFAULT_EXPIRE_DAYS_CRITICAL * 24 * 3600;

// retrieve certificate expiration information and returns info for metric and service check
pub fn inspect_cert(cert: &CertificateDer) -> (ServiceCheckStatus, Option<u64>, Option<u64>, String) {
    match parse_x509_certificate(cert) {
        Ok((_, x509)) => {
            // get certificate remaining time before expiration
            let expiration_timestamp = x509.validity().not_after.timestamp() as u64;
            let current_timestamp = SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap() // TODO: refactor the unwrap to catch the panic case
                .as_secs();

            let seconds_left = expiration_timestamp - current_timestamp;
            let days_left = seconds_left / (24 * 3600);

            // compare the time left before expiration to thresholds and return the corresponding service check
            // TODO: check for several variables setup before taking the constants (need to pass an instance like in python to retrieve custom variables)
            let seconds_warning = DEFAULT_EXPIRE_WARNING as u64;
            let seconds_critical = DEFAULT_EXPIRE_CRITICAL as u64;

            if seconds_left < seconds_critical {
                (
                    ServiceCheckStatus::CRITICAL,
                    Some(days_left),
                    Some(seconds_left),
                    format!("This cert TTL is critical: only {days_left} days before it expires"),
                )
            } else if seconds_left < seconds_warning {
                (
                    ServiceCheckStatus::WARNING,
                    Some(days_left),
                    Some(seconds_left),
                    format!("This cert is almost expired, only {days_left} days left"),
                )
            } else {
                (
                    ServiceCheckStatus::OK,
                    Some(days_left),
                    Some(seconds_left),
                    format!("Days left: {days_left}"),
                )
            }
        }
        Err(e) => {
            (
                ServiceCheckStatus::UNKNOWN,
                None,
                None,
                e.to_string(),
            )
        },
    }
}
