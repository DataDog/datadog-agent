use std::collections::HashMap;
use std::fs::File;
use std::io::Read;
use std::path::PathBuf;
use std::time::{Duration, Instant, SystemTime};

use anyhow::Context;
use bytes::Bytes;
use http::uri::Scheme;
use regex::Regex;

use thiserror::Error;

use http::{HeaderMap, HeaderValue, Method, Request, Response, uri};
use http_body_util::{BodyExt, Full};
use x509_cert::der::Decode;

use hyper::body::{self, Incoming};
use hyper_tls::{HttpsConnector, native_tls};

use hyper_util::client::legacy::{Client, connect::HttpConnector};
use hyper_util::rt::TokioExecutor;

use tokio::net::TcpStream;
use tokio::time;

use super::config;
use super::config::defaults;
use crate::check::config::defaults::REVERSE_CONTENT_MATCH;
use crate::*;

use crate::sink::log;
use crate::sink::service_check::{self, ServiceCheck, Status};
use crate::sink::{Sink, metric};

const MAX_CONTENT_LEN: usize = 20;
const SUPPORTED_SCHEME: [&str; 2] = ["http", "https"];
const DATA_METHODS: [http::Method; 5] = [
    Method::POST,
    Method::PUT,
    Method::DELETE,
    Method::PATCH,
    Method::OPTIONS,
];
const MESSAGE_LENGTH: usize = 2500;

#[derive(Error, Debug)]
pub enum IOErr {
    #[error("connection error")]
    Connect(GenericError),
    #[error("connection timeout")]
    Timeout,
    #[error("error: {0}")]
    Generic(GenericError),
}

enum SvcCheckEvent {
    Status,
    SSLCert,
}

enum SvcCheckMessage {
    WithContent(String),
    WithoutContent(String)
}
struct LightServiceCheck {
    event: SvcCheckEvent,
    status: service_check::Status,
    message: SvcCheckMessage,
}

pub struct HttpCheck<'a, S: Sink> {
    sink: &'a S,
    check_id: String,
    instance_config: config::Instance,
    init_config: config::Init,
    service_checks: Vec<LightServiceCheck>,
    tags: HashMap<String, String>,
}

impl<'a, S: Sink> HttpCheck<'a, S> {
    pub fn new(sink: &'a S, check_id: String, init_config: config::Init, instance_config: config::Instance) -> Self {
        Self {
            sink,
            check_id,
            instance_config,
            init_config,
            service_checks: vec![],
            tags: HashMap::<String, String>::new(),
        }
    }

    pub async fn check(&mut self) {
        if let Err(err) = self.check_impl().await {
            self.sink.log(log::Level::Error, err.to_string())
        }
    }

    async fn check_impl(&mut self) -> Result<()> {
        let url = self.instance_config.url.clone();
        let valid_url = url
            .scheme_str()
            .is_some_and(|s| SUPPORTED_SCHEME.contains(&s))
            && url.host().is_some();
        if !valid_url {
            bail!("Invalid URL: {}", url);
        }

        let mut service_tags = HashMap::<String, String>::new();
        if let Some(tags) = &self.instance_config.tags {
            self.tags = tags.clone();
            service_tags = tags.clone();
        }

        let normalized_name = normalize_tag(&self.instance_config.name);
        self.tags
            .insert("instance".to_string(), normalized_name.clone());
        service_tags.insert("instance".to_string(), normalized_name);

        if !self.tags.contains_key("url") {
            self.tags.insert("url".to_string(), self.instance_config.url.to_string());
        }
        if !service_tags.contains_key("url") {
            service_tags.insert("url".to_string(), self.instance_config.url.to_string());
        }

        if url.scheme_str().is_some_and(|s| s == "https")
            && self.instance_config.tls_verify.is_some_and(|v| !v)
            && !self.instance_config.tls_ignore_warning.is_some_and(|v| v)
            {
            self.sink.log(log::Level::Debug, format!("An unverified HTTPS request is being made to {}", self.instance_config.url))
        }

        let tls = self.make_tls_connector()?; // TODO don't need it for http
        let request = self.make_request()?;

        self.sink
            .log(log::Level::Debug, format!("Connecting to {url}"));

        let start_time = Instant::now();
        let elapsed = || Instant::now().duration_since(start_time);

        let maybe_response = self.http(tls, request).await;
        if let Err(err) = maybe_response.as_ref() {
            let elapsed = elapsed().as_millis();
            self.sink.log(
                log::Level::Info,
                format!(
                    "{} is DOWN, error: {}. Connection failed after {} ms",
                    self.instance_config.url.to_string(),
                    err.to_string(),
                    elapsed
                ),
            );
            self.add_service_check(
                SvcCheckEvent::Status,
                service_check::Status::Critical,
                SvcCheckMessage::WithoutContent(format!(
                    "{}. Connection failed after {} ms",
                    err.to_string(),
                    elapsed
                )) // TODO capitalize first later
            );
        }

        if let Ok((mut response, maybe_certificate)) = maybe_response {
            let total_time = elapsed();

            if self.instance_config
                .collect_response_time
                .unwrap_or(defaults::COLLECT_RESPONSE_TIME)
            {
                self.gauge(
                    "network.http.response_time",
                    (total_time.as_millis() as f64) / 1000.,
                )
            }

            if let Err(err) = self.handle_response(&mut response).await {
                self.sink.log(
                    log::Level::Error,
                    format!(
                        "Error reading response: {}. Connection failed after {} ms",
                        err.to_string(),
                        total_time.as_millis()
                    ),
                )
            }

            let success = self.service_checks[0].status == Status::Ok;
            let can_status = if success { 1. } else { 0. };
            let cant_status = if success { 0. } else { 1. };
            self.gauge("network.http.can_connect", can_status);
            self.gauge("network.http.cant_connect", cant_status);

            if self.instance_config
                .check_certificate_expiration
                .unwrap_or(defaults::CHECK_CERTIFICATE_EXPIRATION)
            {
                self.check_certificate(maybe_certificate)
            }
        }

        let svc = std::mem::replace(&mut self.service_checks, vec![]);
        svc.into_iter().for_each(|lsc| {
            let sc= to_service_check(lsc, &self.check_id, &service_tags);
            if let Err(err) = self.sink.submit_service_check(sc) {
                self.sink
                    .log(log::Level::Error, format!("submit service check: {}", err))
            }
        });

        Ok(())
    }

    async fn http(
        &mut self,
        tls: tokio_native_tls::TlsConnector,
        request: Request<Full<Bytes>>,
    ) -> std::result::Result<(Response<body::Incoming>, Option<native_tls::Certificate>), IOErr>
    {
        let url = self.instance_config.url.clone();
        let port = port_or_default(&url);
        let endpoint = format!("{}:{}", url.host().unwrap(), port);
        let is_https = url.scheme().is_some_and(|s| s == &Scheme::HTTPS);

        let global_timeout = self
            .init_config
            .timeout
            .map_or(defaults::TIMEOUT, Duration::from_secs);
        let connect_timeout = self.instance_config
            .connect_timeout
            .map_or(global_timeout, Duration::from_secs);

        let start_time = Instant::now();
        let remaining_timeout = || connect_timeout - start_time.elapsed();

        let tcp_stream = time::timeout(connect_timeout, async {
            TcpStream::connect(endpoint)
                .await
                .map_err(|e| IOErr::Connect(e.into()))
        })
        .await
        .map_err(|_| IOErr::Timeout)??;

        let maybe_tls = if is_https {
            let tls = time::timeout(remaining_timeout(), async {
                tls.connect(url.host().unwrap(), tcp_stream)
                    .await
                    .map_err(|e| IOErr::Connect(e.into()))
            })
            .await
            .map_err(|_| IOErr::Timeout)??;
            Some(tls)
        } else {
            None
        };

        let read_timeout = self.instance_config
            .read_timeout
            .map_or(remaining_timeout(), Duration::from_secs);

        let mut http = HttpConnector::new();
        http.enforce_http(url.scheme().is_some_and(|s| s == &Scheme::HTTP));

        let https = HttpsConnector::from((http, tls));
        let client = Client::builder(TokioExecutor::new()).build::<_, Full<Bytes>>(https);

        let maybe_response = time::timeout(read_timeout, client.request(request))
            .await
            .map_err(|_| IOErr::Timeout)?;

        let mut certificate: Option<native_tls::Certificate> = None;
        if is_https {
            match maybe_tls.unwrap().get_ref().peer_certificate() {
                Ok(cert) => certificate = cert,
                Err(err) => self.sink.log(
                    log::Level::Error,
                    format!("Read peer certificate: {}", err.to_string()),
                ),
            }
        }

        let response = maybe_response.map_err(|e| IOErr::Generic(e.into()))?;
        Ok((response, certificate))
    }

    fn make_tls_connector(&self) -> Result<tokio_native_tls::TlsConnector> {
        let mut tls_builder = native_tls::TlsConnector::builder();
        tls_builder.danger_accept_invalid_certs(!self.instance_config.tls_verify.unwrap_or(defaults::TLS_VERIFY));

        if let Some(path) = self.instance_config.tls_cert.as_ref() {
            tls_builder.disable_built_in_roots(true);
            let cert = load_pem(path)?;
            tls_builder.add_root_certificate(cert);
        }

        let native_tls = tls_builder.build()?;

        Ok(tokio_native_tls::TlsConnector::from(native_tls))
    }

    fn make_request(&self) -> Result<Request<Full<Bytes>>> {
        let mut headers = HeaderMap::new();
        if let Some(h) = &self.instance_config.headers {
            headers = h.clone()
        }

        let method = self.instance_config.method.as_ref().unwrap_or(&defaults::METHOD);
        if DATA_METHODS.contains(method) && !headers.contains_key("Content-Type") {
            headers.insert(
                "Content-Type",
                HeaderValue::from_static("application/x-www-form-urlencoded"),
            );
        }

        let mut request = http::Request::builder()
            .method(self.instance_config.method.as_ref().unwrap_or(&defaults::METHOD).clone())
            .uri(&self.instance_config.url);
        *request.headers_mut().unwrap() = headers; // FIXME unwrap

        let body = match &self.instance_config.data {
            Some(data) => Full::from(data.clone()),
            _ => Full::new(Bytes::new()),
        };

        Ok(request.body(body)?)
    }

    fn check_certificate(
        &mut self,
        maybe_certificate: Option<native_tls::Certificate>,
    ) {
        let certificate = match maybe_certificate {
            Some(cert) => cert,
            None => {
                self.ssl_service_check(
                    Status::Unknown,
                    "Empty or no certificate found.".to_string(),
                );
                return;
            }
        };

        // FIXME can this conversion be avoided?
        let certificate = match certificate
            .to_der()
            .map_err(GenericError::from)
            .and_then(|der| x509_cert::Certificate::from_der(&der).map_err(GenericError::from))
        {
            Ok(cert) => cert,
            Err(err) => {
                self.ssl_service_check(
                    Status::Unknown,
                    format!(
                        "Unable to parse the certificate to get expiration: {}",
                        err.to_string()
                    ),
                );
                return;
            }
        };

        let not_after = certificate
            .tbs_certificate
            .validity
            .not_after
            .to_system_time();

        let warning = Duration::from_secs(
            self.instance_config.seconds_warning
                .unwrap_or(self.instance_config.days_warning.unwrap_or(defaults::DAYS_WARNING) * 24 * 60 * 60),
        );
        let critical = Duration::from_secs(
            self.instance_config.seconds_critical
                .unwrap_or(self.instance_config.days_warning.unwrap_or(defaults::DAYS_WARNING) * 24 * 60 * 60),
        );

        let to_days = |d: Duration| d.as_secs() / 60 / 60 / 24;

        match not_after.duration_since(SystemTime::now()) {
            Ok(left) => {
                self.gauge("http.ssl.days_left", to_days(left) as f64);
                self.gauge("http.ssl.seconds_left", left.as_secs() as f64);
                if left < critical {
                    self.ssl_service_check(
                        service_check::Status::Critical,
                        format!(
                            "This cert TTL is critical: only {} days before it expires",
                            to_days(left)
                        )
                    )
                } else if left < warning {
                    self.ssl_service_check(
                        service_check::Status::Critical,
                        format!(
                            "This cert is almost expired, only {} days left",
                            to_days(left)
                        )
                    )
                } else {
                    self.ssl_service_check(
                        service_check::Status::Ok,
                        format!("Days left: {}", to_days(left)),
                    )
                }
            }
            Err(_) => {
                self.gauge("http.ssl.days_left", 0.);
                self.gauge("http.ssl.seconds_left", 0.);
                self.ssl_service_check(
                    service_check::Status::Critical,
                    "This cert is expired".to_string()
                )
            }
        }
    }

    async fn handle_response(
        &mut self,
        response: &mut Response<Incoming>,
    ) -> Result<()> {
        let mut body = Vec::<u8>::with_capacity(MAX_CONTENT_LEN);
        while let Some(frame) = response.body_mut().frame().await {
            let frame = frame?;

            if let Some(d) = frame.data_ref() {
                body.extend_from_slice(d.as_ref());
            }
            // FIXME don't read more than MAX_CONTENT_LEN
            if body.len() >= MAX_CONTENT_LEN {
                break;
            }
        }
        let body = String::from_utf8_lossy(&body);

        let maybe_content = |mut msg| {
            if self.instance_config.include_content.unwrap_or(defaults::INCLUDE_CONTENT) {
                msg += "\nContent: ";
                msg +=  &body[..MESSAGE_LENGTH.min(body.len())];
                SvcCheckMessage::WithContent(msg)
            } else {
                SvcCheckMessage::WithoutContent(msg)
            }
        };
        let get_message = |msg: &SvcCheckMessage| {
            match msg {
                SvcCheckMessage::WithContent(msg) => msg,
                SvcCheckMessage::WithoutContent(msg) => msg
            }.clone()
        };

        let pattern = match self.instance_config.http_response_status_code.as_ref() {
            Some(s) => &s,
            None => defaults::HTTP_RESPONSE_STATUS_CODE,
        };
        let regex = Regex::new(pattern)?;

        if !regex.is_match(response.status().as_str()) {
            let message = maybe_content(format!(
                "Incorrect HTTP return code for url {}. Expected {}, got {}.",
                self.instance_config.url,
                pattern,
                response.status().as_str()
            ));
            self.sink.log(log::Level::Info, get_message(&message));
            self.add_service_check(
                SvcCheckEvent::Status,
                service_check::Status::Critical,
                message,
            );
            return Ok(());
        }

        if let Some(needle) = self.instance_config.content_match.as_ref() {
            let reverse = self.instance_config.reverse_content_match.unwrap_or(REVERSE_CONTENT_MATCH);
            let regex = Regex::new(&needle)?;
            if regex.is_match(&body) {
                if reverse {
                    self.send_status_down(
                        format!(
                            "{} is found in return content with the reverse_content_match option",
                            needle
                        ),
                        maybe_content(format!(
                            "Content \"{}\" found in response with the reverse_content_match",
                            needle
                        ))
                    )
                } else {
                    self.send_status_up(format!("{} is found in return content ", needle))
                }
            } else {
                if reverse {
                    self.send_status_up(format!(
                        "{} is not found in return content with the reverse_content_match option",
                        needle
                    ))
                } else {
                    self.send_status_down(
                        format!("{} is not found in return content", needle),
                        maybe_content(format!(
                            "Content \"{}\" not found in response.",
                            needle)
                        )
                    )
                }
            }
        } else {
            self.send_status_up(format!("{} is UP", self.instance_config.url)) // FIXME addr
        }

        Ok(())
    }

    fn add_service_check(
        &mut self,
        event: SvcCheckEvent,
        status: service_check::Status,
        message: SvcCheckMessage,
    ) {
        let lsc = LightServiceCheck {
            event,
            status,
            message
        };
        self.service_checks.push(lsc)
    }

    fn ssl_service_check(&mut self, status: service_check::Status, message: String) {
        self.add_service_check(SvcCheckEvent::SSLCert, status, SvcCheckMessage::WithoutContent(message))
    }

    fn gauge(&self, name: &str, value: f64) {
        let res = self.sink.submit_metric(
            metric::Metric {
                id: self.check_id.clone(),
                metric_type: metric::Type::Gauge,
                name: name.to_string(),
                value: value,
                tags: self.tags.clone(),
                hostname: String::new(),
            },
            false,
        );
        if let Err(err) = res {
            self.sink
                .log(log::Level::Error, format!("submit metric: {}", err))
        }
    }

    fn send_status_up(&mut self, message: String) {
        self.sink.log(log::Level::Debug, message);
        self.add_service_check(
            SvcCheckEvent::Status,
            service_check::Status::Ok,
            SvcCheckMessage::WithoutContent("UP".to_string())
        )
    }

    fn send_status_down(&mut self, log_msg: String, down_msg: SvcCheckMessage) {
        self.sink.log(log::Level::Info, log_msg);
        self.add_service_check(
            SvcCheckEvent::Status,
            service_check::Status::Critical,
            down_msg,
        )
    }
}

fn port_or_default(uri: &uri::Uri) -> u16 {
    match uri.port() {
        Some(port) => port.as_u16(),
        None => match uri.scheme_str() {
            // FIXME why can't use scheme()?
            Some("http") => 80,
            Some("https") => 443,
            _ => panic!("unexpected scheme"),
        },
    }
}

fn load_pem(path: &PathBuf) -> Result<native_tls::Certificate> {
    let mut file =
        File::open(path).with_context(|| format!("opening {} certificate", path.display()))?;
    let mut buffer = Vec::<u8>::new();
    file.read_to_end(&mut buffer)
        .with_context(|| format!("reading {} certificate", path.display()))?;
    native_tls::Certificate::from_pem(&buffer).map_err(|e| anyhow!("parsing certificate: {}", e))
}

fn normalize_tag(tag: &str) -> String {
    let tag_replacement = Regex::new(r#"[,\+\*\-/()\[\]{}\s]"#).expect("invalid regex");
    let multiple_underscore_cleanup = Regex::new(r#"__+"#).expect("invalid regex");
    let dot_underscore_cleanup = Regex::new(r#"_*\._*"#).expect("invalid regex");

    let tag = tag_replacement.replace_all(tag, "_");
    let tag = multiple_underscore_cleanup.replace_all(&tag, "_");
    let tag = dot_underscore_cleanup.replace_all(&tag, ".");
    tag.trim_matches('_').to_string()
}

fn to_service_check(lsc: LightServiceCheck, check_id: &str, tags: &HashMap<String,String>) -> ServiceCheck {
    let event = match lsc.event {
        SvcCheckEvent::Status => "http.can_connect",
        SvcCheckEvent::SSLCert => "http.ssl_cert",
    };
    let message = match lsc.message {
        SvcCheckMessage::WithoutContent(msg) => msg,
        SvcCheckMessage::WithContent(mut msg) => {
            if msg.len() > 20 {
                msg.replace_range(17.., "...");
            };
            msg
        }
    };
    ServiceCheck {
        id: check_id.to_string(),
        name: event.to_string(),
        status: lsc.status,
        tags: tags.clone(),
        hostname: String::new(),
        message: message
    }
}