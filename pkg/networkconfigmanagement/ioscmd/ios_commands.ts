/**
 * JSON schema (as TypeScript types) for a block of Cisco IOS / IOS-XE commands.
 *
 * A document is an ordered list of top-level commands. Entering and leaving
 * configuration mode (`configure terminal` / `end`) is implicit: the renderer
 * (the Go code in this package) inserts them as needed between `show`
 * commands and configuration commands.
 *
 * Example:
 *
 *   show running-config | section system
 *   hostname mydevice
 *   default interface GigabitEthernet1/0/1
 *   interface GigabitEthernet1/0/1
 *    shutdown
 *    description Some description
 *    no switchport
 *    ip address 192.0.2.1 255.255.255.252
 *    no shutdown
 *   end
 *
 * is represented as:
 *
 *   [
 *     { type: "show", target: "running-config", pipe: { type: "section", section: "system" } },
 *     { type: "hostname", hostname: "mydevice" },
 *     {
 *       type: "interface",
 *       name: "GigabitEthernet1/0/1",
 *       default_first: true,
 *       commands: [
 *         { type: "shutdown", value: true },
 *         { type: "description", value: "Some description" },
 *         { type: "switchport", value: false },
 *         { type: "ip_addr", addr: "192.0.2.1", mask: "255.255.255.252" },
 *         { type: "shutdown", value: false },
 *       ],
 *     },
 *   ]
 */

// ---------------------------------------------------------------------------
// Primitive aliases (documentation only; validated at runtime by the executor)
// ---------------------------------------------------------------------------

/** Dotted-quad IPv4 address, e.g. "192.0.2.1". */
export type IPv4Address = string;

/** Dotted-quad IPv4 subnet mask, e.g. "255.255.255.252". */
export type IPv4Mask = string;

/** IPv6 prefix in CIDR notation, e.g. "2001:db8::1/64". */
export type IPv6Prefix = string;

/**
 * Full or abbreviated IOS interface name, e.g. "GigabitEthernet1/0/1", "Vlan10",
 * "Loopback0", "Port-channel1", "Gi1/0/1.100", "Serial0/0/0:0". Must match
 * /^[A-Za-z][A-Za-z-]*[0-9][0-9\/.:]*$/ (no spaces).
 */
export type InterfaceName = string;

/** Device hostname: letters, digits, and hyphens, starting with a letter, at most 63 characters. */
export type Hostname = string;

/** VRF or ACL name: letters, digits, "_", ".", and "-", at most 64 characters. */
export type ObjectName = string;

/**
 * Free-form text (descriptions, pipe regexes): printable ASCII, not blank, and
 * without "|" or "?", which the CLI would interpret as a pipe or a help request.
 * Regex alternation ("a|b") is therefore not supported.
 */
export type CliText = string;

/** VLAN ID, 1-4094. */
export type VlanId = number;

/** VLAN list in IOS syntax, e.g. "10,20,30-40". */
export type VlanList = string;

// ---------------------------------------------------------------------------
// Document root
// ---------------------------------------------------------------------------

/** A full command block: an ordered list of top-level commands. */
export type CommandBlock = TopLevelCommand[];

export type TopLevelCommand = ShowCommand | HostnameCommand | InterfaceCommand;

// ---------------------------------------------------------------------------
// Show commands
// ---------------------------------------------------------------------------

/** Output filter applied with `| <modifier> <arg>`. */
export type OutputPipe =
  /** `| section <regex>` */
  | { type: "section"; section: CliText }
  /** `| include <regex>` */
  | { type: "include"; pattern: CliText }
  /** `| exclude <regex>` */
  | { type: "exclude"; pattern: CliText }
  /** `| begin <regex>` */
  | { type: "begin"; pattern: CliText }
  /** `| count <regex>` */
  | { type: "count"; pattern: CliText };

interface ShowBase {
  type: "show";
  pipe?: OutputPipe;
}

/** `show running-config [interface <name>]` / `show startup-config` */
export type ShowConfig =
  | (ShowBase & {
      target: "running-config";
      /** Restrict output to a single interface (`show running-config interface X`). */
      interface?: InterfaceName;
    })
  | (ShowBase & { target: "startup-config" });

/** `show interfaces [<name>] [status | description | counters]` */
export interface ShowInterfaces extends ShowBase {
  target: "interfaces";
  interface?: InterfaceName;
  detail?: "status" | "description" | "counters" | "switchport" | "trunk";
}

/** `show ip interface [brief | <name>]` (brief and interface are mutually exclusive) */
export type ShowIpInterface = ShowBase & { target: "ip interface" } & (
    | { brief?: boolean; interface?: never }
    | { brief?: false; interface?: InterfaceName }
  );

/** `show ip route [vrf <vrf>] [<prefix>]` */
export interface ShowIpRoute extends ShowBase {
  target: "ip route";
  vrf?: ObjectName;
  prefix?: IPv4Address;
}

/** `show vlan [brief | id <vlan>]` (brief and id are mutually exclusive) */
export type ShowVlan = ShowBase & { target: "vlan" } & (
    | { brief?: boolean; id?: never }
    | { brief?: false; id?: VlanId }
  );

/** `show cdp neighbors [detail]` / `show lldp neighbors [detail]` */
export interface ShowNeighbors extends ShowBase {
  target: "cdp neighbors" | "lldp neighbors";
  detail?: boolean;
}

/** `show mac address-table [interface <name> | vlan <id>]` (interface and vlan are mutually exclusive) */
export type ShowMacAddressTable = ShowBase & { target: "mac address-table" } & (
    | { interface?: InterfaceName; vlan?: never }
    | { interface?: never; vlan?: VlanId }
  );

/** Show commands that take no arguments. */
export interface ShowSimple extends ShowBase {
  target: "version" | "inventory" | "clock" | "logging" | "users" | "boot";
}

export type ShowCommand =
  | ShowConfig
  | ShowInterfaces
  | ShowIpInterface
  | ShowIpRoute
  | ShowVlan
  | ShowNeighbors
  | ShowMacAddressTable
  | ShowSimple;

// ---------------------------------------------------------------------------
// Global configuration commands
// ---------------------------------------------------------------------------

/** `hostname <name>` */
export interface HostnameCommand {
  type: "hostname";
  hostname: Hostname;
}

/**
 * `interface <name>` followed by indented sub-commands.
 *
 * If `default_first` is true, `default interface <name>` is issued before
 * entering the interface, resetting it to factory defaults.
 */
export interface InterfaceCommand {
  type: "interface";
  name: InterfaceName;
  default_first?: boolean;
  commands: InterfaceSubCommand[];
}

// ---------------------------------------------------------------------------
// Interface sub-commands
// ---------------------------------------------------------------------------

/**
 * Boolean toggles. `value: true` emits `<keyword>`, `value: false` emits
 * `no <keyword>`.
 */
export interface ToggleCommand {
  type:
    | "shutdown" // shutdown
    | "switchport" // switchport
    | "negotiation_auto" // negotiation auto
    | "cdp_enable" // cdp enable
    | "lldp_transmit" // lldp transmit
    | "lldp_receive" // lldp receive
    | "spanning_tree_portfast" // spanning-tree portfast
    | "spanning_tree_bpduguard" // spanning-tree bpduguard enable
    | "ip_redirects" // ip redirects
    | "ip_proxy_arp" // ip proxy-arp
    | "ipv6_enable"; // ipv6 enable
  value: boolean;
}

/** `description <text>`; `value: null` emits `no description`. */
export interface DescriptionCommand {
  type: "description";
  value: CliText | null;
}

/** `ip address <addr> <mask> [secondary]` */
export interface IpAddressCommand {
  type: "ip_addr";
  addr: IPv4Address;
  mask: IPv4Mask;
  secondary?: boolean;
}

/** `ip address dhcp` */
export interface IpAddressDhcpCommand {
  type: "ip_addr_dhcp";
}

/** `no ip address` (removes all IPv4 addresses). */
export interface NoIpAddressCommand {
  type: "no_ip_addr";
}

/**
 * `ipv6 address <prefix> [eui-64]` or `ipv6 address <addr> link-local`;
 * `remove` emits the `no` form. With `link-local`, `prefix` is a bare
 * fe80::/10 address with no prefix length, e.g. "fe80::1".
 */
export interface Ipv6AddressCommand {
  type: "ipv6_addr";
  prefix: IPv6Prefix;
  modifier?: "link-local" | "eui-64";
  remove?: boolean;
}

/** `ip helper-address <addr>`; `remove` emits the `no` form. */
export interface IpHelperAddressCommand {
  type: "ip_helper_address";
  addr: IPv4Address;
  remove?: boolean;
}

/** `vrf forwarding <name>`; `value: null` emits `no vrf forwarding`. */
export interface VrfForwardingCommand {
  type: "vrf_forwarding";
  value: ObjectName | null;
}

/** `switchport mode <mode>` */
export interface SwitchportModeCommand {
  type: "switchport_mode";
  mode: "access" | "trunk" | "dynamic auto" | "dynamic desirable";
}

/** `switchport access vlan <id>`; `value: null` emits the `no` form. */
export interface SwitchportAccessVlanCommand {
  type: "switchport_access_vlan";
  value: VlanId | null;
}

/** `switchport voice vlan <id>`; `value: null` emits the `no` form. */
export interface SwitchportVoiceVlanCommand {
  type: "switchport_voice_vlan";
  value: VlanId | null;
}

/** `switchport trunk native vlan <id>`; `value: null` emits the `no` form. */
export interface SwitchportTrunkNativeVlanCommand {
  type: "switchport_trunk_native_vlan";
  value: VlanId | null;
}

/** `switchport trunk allowed vlan [add | remove | except] <list>` / `... all` / `... none` */
export interface SwitchportTrunkAllowedVlanCommand {
  type: "switchport_trunk_allowed_vlan";
  action?: "set" | "add" | "remove" | "except";
  vlans: VlanList | "all" | "none";
}

/** `speed <value>`; `value: null` emits `no speed`. */
export interface SpeedCommand {
  type: "speed";
  value: "auto" | "10" | "100" | "1000" | "2500" | "5000" | "10000" | "25000" | "40000" | "100000" | null;
}

/** `duplex <value>`; `value: null` emits `no duplex`. */
export interface DuplexCommand {
  type: "duplex";
  value: "auto" | "full" | "half" | null;
}

/** `mtu <bytes>`; `value: null` emits `no mtu`. */
export interface MtuCommand {
  type: "mtu";
  value: number | null;
}

/** `ip mtu <bytes>`; `value: null` emits `no ip mtu`. */
export interface IpMtuCommand {
  type: "ip_mtu";
  value: number | null;
}

/** `channel-group <n> mode <mode>`; `value: null` emits `no channel-group`. */
export interface ChannelGroupCommand {
  type: "channel_group";
  value: { group: number; mode: "active" | "passive" | "on" | "auto" | "desirable" } | null;
}

/** `ip access-group <acl> {in | out}`; `remove` emits the `no` form. */
export interface IpAccessGroupCommand {
  type: "ip_access_group";
  acl: ObjectName;
  direction: "in" | "out";
  remove?: boolean;
}

export type InterfaceSubCommand =
  | ToggleCommand
  | DescriptionCommand
  | IpAddressCommand
  | IpAddressDhcpCommand
  | NoIpAddressCommand
  | Ipv6AddressCommand
  | IpHelperAddressCommand
  | VrfForwardingCommand
  | SwitchportModeCommand
  | SwitchportAccessVlanCommand
  | SwitchportVoiceVlanCommand
  | SwitchportTrunkNativeVlanCommand
  | SwitchportTrunkAllowedVlanCommand
  | SpeedCommand
  | DuplexCommand
  | MtuCommand
  | IpMtuCommand
  | ChannelGroupCommand
  | IpAccessGroupCommand;
