// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

use std::collections::{BTreeSet, HashMap};

use crate::netns::{self, Ino, NamespaceInfo, Port};

const MAX_PORTS_NUMBER: usize = 50;

pub fn get(
    context: &mut ParsingContext,
    pid: i32,
    sockets: &[u64],
) -> (Option<Vec<Port>>, Option<Vec<Port>>) {
    if sockets.is_empty() {
        return (None, None);
    }

    let Ok(netns_ino) = netns::get_netns_ino(pid) else {
        return (None, None);
    };

    // The socket and network address information are different for each
    // network namespace.  Since namespaces can be shared between multiple
    // processes, we cache them to only parse them once per call to this
    // function.
    let netns_info = context
        .netns_info
        .entry(netns_ino)
        .or_insert(netns::get_netns_info(pid));

    let mut tcp_ports = BTreeSet::new();
    let mut udp_ports = BTreeSet::new();

    for socket in sockets {
        if let Some(port) = netns_info.tcp_sockets.get(socket).copied() {
            tcp_ports.insert(port);
        } else if let Some(port) = netns_info.udp_sockets.get(socket).copied() {
            udp_ports.insert(port);
        }
    }

    let tcp_ports = if tcp_ports.is_empty() {
        None
    } else {
        Some(tcp_ports.into_iter().take(MAX_PORTS_NUMBER).collect())
    };

    let udp_ports = if udp_ports.is_empty() {
        None
    } else {
        Some(udp_ports.into_iter().take(MAX_PORTS_NUMBER).collect())
    };

    (tcp_ports, udp_ports)
}

pub struct ParsingContext {
    pub netns_info: HashMap<Ino, NamespaceInfo>,
}

impl ParsingContext {
    pub fn new() -> Self {
        ParsingContext {
            netns_info: HashMap::new(),
        }
    }
}
