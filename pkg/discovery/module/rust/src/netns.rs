// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::collections::HashMap;
use std::fs;
use std::io::{BufRead, BufReader, Read};
use std::os::unix::fs::MetadataExt;

use crate::ephemeral::{EphemeralPortType, is_port_ephemeral};
use crate::errors::Error;
use crate::procfs;

pub type Ino = u64;
pub type Port = u16;

pub fn get_netns_ino(pid: i32) -> std::io::Result<Ino> {
    let pid = pid.to_string();
    let pid_netns_path = procfs::root_path().join(pid).join("ns/net");
    let metadata = fs::metadata(pid_netns_path)?;

    Ok(metadata.ino())
}

#[derive(Debug)]
pub struct NamespaceInfo {
    pub tcp_sockets: HashMap<Ino, Port>,
    pub udp_sockets: HashMap<Ino, Port>,
}

impl NamespaceInfo {
    pub fn new() -> Self {
        Self {
            tcp_sockets: HashMap::new(),
            udp_sockets: HashMap::new(),
        }
    }
}

/// Get the list of open ports by parsing the process socket tables.
pub fn get_netns_info(pid: i32) -> NamespaceInfo {
    let mut info = NamespaceInfo::new();

    parse_socket_table(
        pid,
        "tcp",
        SocketTableState::TCPListen,
        &mut info.tcp_sockets,
    );
    parse_socket_table_filtered(
        pid,
        "udp",
        SocketTableState::UDPListen,
        |port| is_port_ephemeral(port) == EphemeralPortType::NotEphemeral,
        &mut info.udp_sockets,
    );
    parse_socket_table(
        pid,
        "tcp6",
        SocketTableState::TCPListen,
        &mut info.tcp_sockets,
    );
    parse_socket_table_filtered(
        pid,
        "udp6",
        SocketTableState::UDPListen,
        |port| is_port_ephemeral(port) == EphemeralPortType::NotEphemeral,
        &mut info.udp_sockets,
    );

    info
}

#[derive(Debug, PartialEq)]
enum SocketTableState {
    UDPListen = 0x07,
    TCPListen = 0x0A,
}

impl TryFrom<u8> for SocketTableState {
    type Error = Error;

    fn try_from(value: u8) -> Result<Self, Self::Error> {
        match value {
            0x07 => Ok(Self::UDPListen),
            0x0A => Ok(Self::TCPListen),
            _ => Err(Error::SocketParsingError {
                context: format!("unknown socket state: 0x{value:02x}"),
            }),
        }
    }
}

/// Parse a socket table file from /proc/<pid>/net and collect opened sockets
/// with the expected state.
fn parse_socket_table(
    pid: i32,
    socket_table: &str,
    expected_state: SocketTableState,
    result: &mut HashMap<Ino, Port>,
) {
    parse_socket_table_filtered(pid, socket_table, expected_state, |_| true, result);
}

/// Parse a socket table file from /proc/<pid>/net and collect opened sockets
/// with the expected state, applying a port filter.
fn parse_socket_table_filtered<F>(
    pid: i32,
    socket_table: &str,
    expected_state: SocketTableState,
    port_filter: F,
    result: &mut HashMap<Ino, Port>,
) where
    F: Fn(Port) -> bool,
{
    const READ_LIMIT: u64 = 4 * 1024 * 1024 * 1024; // 4GiB

    let pid = pid.to_string();

    let sock_table = procfs::root_path().join(pid).join("net").join(socket_table);
    let Ok(sock_table) = fs::File::open(sock_table) else {
        return;
    };
    let sock_table = sock_table.take(READ_LIMIT);
    let mut sock_table = BufReader::new(sock_table);

    let mut line_buf = String::with_capacity(256);

    // Skip the header line
    if sock_table.read_line(&mut line_buf).is_err() {
        return;
    };

    loop {
        line_buf.clear();
        match sock_table.read_line(&mut line_buf) {
            Ok(0) => break, // EOF
            Ok(_) => {
                match parse_socket_line(&line_buf, &expected_state) {
                    Ok(Some((inode, port))) => {
                        if port_filter(port) {
                            result.insert(inode, port);
                        }
                    }
                    Ok(None) | Err(_) => continue,
                };
            }
            Err(_) => break,
        }
    }
}

fn get_fields(line: &str) -> Option<(&str, &str, &str)> {
    let mut iter = line.split_whitespace();
    let local_address = iter.nth(1)?; // field 1: local address
    let state = iter.nth(1)?; // field 3: state (skip field 2)
    let inode = iter.nth(5)?; // field 9: inode (skip fields 4-8)
    Some((local_address, state, inode))
}

fn parse_socket_line(
    line: &str,
    expected_state: &SocketTableState,
) -> Result<Option<(Ino, Port)>, Error> {
    let Some((local_address, state, inode)) = get_fields(line) else {
        return Err(Error::SocketParsingError {
            context: "failed to parse socket line fields".to_string(),
        });
    };

    // Parse state
    let Ok(state) = u8::from_str_radix(state, 16) else {
        return Err(Error::SocketParsingError {
            context: "failed to parse socket state".to_string(),
        });
    };

    let Ok(state) = SocketTableState::try_from(state) else {
        return Ok(None); // Unknown state, skip
    };

    if state != *expected_state {
        return Ok(None);
    }

    // Parse local address - format: "IP:PORT"
    let Some(colon_pos) = local_address.rfind(':') else {
        return Err(Error::SocketParsingError {
            context: "no colon found in local address".to_string(),
        });
    };

    let port = local_address
        .get(colon_pos + 1..)
        .ok_or(Error::SocketParsingError {
            context: "could not extract port number".to_string(),
        })?;
    let Ok(port) = u16::from_str_radix(port, 16) else {
        return Err(Error::SocketParsingError {
            context: "failed to parse port number".to_string(),
        });
    };

    // Parse inode
    let Ok(inode) = inode.parse::<u64>() else {
        return Err(Error::SocketParsingError {
            context: "failed to parse inode".to_string(),
        });
    };

    Ok(Some((inode, port)))
}
