// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::collections::HashMap;
use std::fs;
use std::io::{BufRead, BufReader, Read};
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};
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

/// Returns the inode of the network namespace of the calling process.
///
/// This reads `/proc/self/ns/net` directly instead of the configured proc
/// root: the proc root may point at another PID namespace (e.g. the host's
/// `/host/proc` when running in a container), in which case the calling
/// process is not listed there.
pub fn get_self_netns_ino() -> std::io::Result<Ino> {
    let metadata = fs::metadata("/proc/self/ns/net")?;
    Ok(metadata.ino())
}

/// Returns the addresses each listening TCP port of the network namespace of
/// `pid` is bound to. Like `get_netns_info`, this parses the socket tables
/// under `/proc/<pid>/net`, which are shared by every process in the same
/// network namespace.
pub fn get_listen_addrs(pid: i32) -> HashMap<Port, Vec<IpAddr>> {
    let mut result: HashMap<Port, Vec<IpAddr>> = HashMap::new();
    for table in ["tcp", "tcp6"] {
        parse_listen_addrs_table(pid, table, &mut result);
    }
    result
}

fn parse_listen_addrs_table(
    pid: i32,
    socket_table: &str,
    result: &mut HashMap<Port, Vec<IpAddr>>,
) {
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
                if let Some((port, addr)) = parse_listen_addr_line(&line_buf) {
                    let entry = result.entry(port).or_default();
                    if !entry.contains(&addr) {
                        entry.push(addr);
                    }
                }
            }
            Err(_) => break,
        }
    }
}

/// Parses one line of `/proc/<pid>/net/tcp{,6}` and returns the port and
/// bound address when the socket is in LISTEN state.
fn parse_listen_addr_line(line: &str) -> Option<(Port, IpAddr)> {
    const TCP_LISTEN_STATE: u8 = 0x0A;

    let (local_address, state, _inode) = crate::netns::get_fields(line)?;

    let Ok(state) = u8::from_str_radix(state, 16) else {
        return None;
    };
    if state != TCP_LISTEN_STATE {
        return None;
    }

    let colon_pos = local_address.rfind(':')?;
    let addr_hex = local_address.get(..colon_pos)?;
    let port_hex = local_address.get(colon_pos + 1..)?;
    let port = u16::from_str_radix(port_hex, 16).ok()?;
    let addr = parse_hex_addr(addr_hex)?;

    Some((port, addr))
}

/// Parses the hex-encoded local address of `/proc/net/tcp{,6}`.
///
/// The kernel prints IPv4 addresses as one little-endian 32-bit value and
/// IPv6 addresses as four little-endian 32-bit words (see `tcp4_seq_show`/
/// `tcp6_seq_show` in the Linux kernel).
fn parse_hex_addr(hex: &str) -> Option<IpAddr> {
    match hex.len() {
        8 => {
            let v = u32::from_str_radix(hex, 16).ok()?;
            Some(IpAddr::V4(Ipv4Addr::from(v.to_le_bytes())))
        }
        32 => {
            let mut bytes = [0u8; 16];
            for (i, word) in hex.as_bytes().chunks(8).enumerate() {
                let word = std::str::from_utf8(word).ok()?;
                let v = u32::from_str_radix(word, 16).ok()?;
                let le = v.to_le_bytes();
                let slot = bytes.get_mut(i * 4..i * 4 + 4)?;
                slot.copy_from_slice(&le);
            }
            Some(IpAddr::V6(Ipv6Addr::from(bytes)))
        }
        _ => None,
    }
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

#[cfg(test)]
#[allow(clippy::unwrap_used, clippy::expect_used)]
mod addr_tests {
    use super::*;

    #[test]
    fn test_parse_hex_addr_ipv4() {
        // 127.0.0.1 as printed by /proc/net/tcp
        assert_eq!(
            parse_hex_addr("0100007F"),
            Some(IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)))
        );
        // 0.0.0.0
        assert_eq!(
            parse_hex_addr("00000000"),
            Some(IpAddr::V4(Ipv4Addr::UNSPECIFIED))
        );
        // 10.128.68.59 = 0A 80 44 3B in network order; the kernel prints the
        // little-endian u32 read of those bytes: 0x3B44800A
        assert_eq!(
            parse_hex_addr("3B44800A"),
            Some(IpAddr::V4(Ipv4Addr::new(10, 128, 68, 59)))
        );
    }

    #[test]
    fn test_parse_hex_addr_ipv6() {
        // ::1
        assert_eq!(
            parse_hex_addr("00000000000000000000000001000000"),
            Some(IpAddr::V6(Ipv6Addr::LOCALHOST))
        );
        // ::
        assert_eq!(
            parse_hex_addr("00000000000000000000000000000000"),
            Some(IpAddr::V6(Ipv6Addr::UNSPECIFIED))
        );
        // ::ffff:127.0.0.1 (IPv4-mapped) = bytes 0-9 zero, 10-11 = FF FF,
        // 12-15 = 7F 00 00 01. Word 2 (bytes 8-11 = 00 00 FF FF) is printed
        // as the little-endian u32 0xFFFF0000, word 3 (7F 00 00 01) as
        // 0x0100007F.
        assert_eq!(
            parse_hex_addr("0000000000000000FFFF00000100007F"),
            Some(IpAddr::V6(Ipv6Addr::from([
                0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xFF, 0xFF, 127, 0, 0, 1
            ])))
        );
    }

    #[test]
    fn test_parse_hex_addr_invalid() {
        assert_eq!(parse_hex_addr("12345"), None);
        assert_eq!(parse_hex_addr(""), None);
        assert_eq!(parse_hex_addr("GGGGGGGG"), None);
    }

    #[test]
    fn test_parse_listen_addr_line() {
        // Fields: sl local_address rem_address st ...
        let line = "  1: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0\r\n";
        let (port, addr) = parse_listen_addr_line(line).expect("parse");
        assert_eq!(port, 8080); // 0x1F90
        assert_eq!(addr, IpAddr::V4(Ipv4Addr::new(127, 0, 0, 1)));

        // Established (state 01) must be ignored.
        let established = "  2: 0100007F:1F90 0100007F:CAF3 01 00000000:00000000 02:0006B02B 00000000     0        0 22222 1 0000000000000000 20 4 30 10 -1\r\n";
        assert!(parse_listen_addr_line(established).is_none());
    }
}
