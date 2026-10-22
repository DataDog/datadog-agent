/**
 * Types to describe device commands.
 *
 * Currently this supports a small subset of Cisco IOS CLI commands.
 */

// ------------------------------------------------------------------------
// Primitive aliases (documentation only; validated at runtime by executor)
// ------------------------------------------------------------------------

/**
 * Dotted-quad IPv4 address, e.g. "192.0.2.1".
 * @pattern ^((25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.){3}(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])$
 */
export type IPv4Address = string;

/**
 * Dotted-quad IPv4 subnet mask, e.g. "255.255.255.252".
 * @pattern ^(255\.255\.255\.(255|254|252|248|240|224|192|128|0)|255\.255\.(254|252|248|240|224|192|128|0)\.0|255\.(254|252|248|240|224|192|128|0)\.0\.0|(254|252|248|240|224|192|128|0)\.0\.0\.0)$
 */
export type IPv4Mask = string;

/**
 * IPv6 prefix in CIDR notation, e.g. "2001:db8::1/64".
 * @pattern ^[0-9A-Fa-f]*:[0-9A-Fa-f:.]*(/(12[0-8]|1[01][0-9]|[1-9]?[0-9]))?$
 * @maxLength 49
 */
export type IPv6Prefix = string;

/**
 * Full or abbreviated IOS interface name, e.g. "GigabitEthernet1/0/1", "Vlan10",
 * "Loopback0", "Port-channel1", "Gi1/0/1.100", "Serial0/0/0:0". Must match
 * /^[A-Za-z][A-Za-z-]*[0-9][0-9\/.:]*$/ (no spaces).
 * @pattern ^[A-Za-z][A-Za-z-]*[0-9][0-9/.:]*$
 */
export type InterfaceName = string;

/**
 * Device hostname: letters, digits, and hyphens, starting with a letter, at most 63 characters.
 * @pattern ^[A-Za-z][A-Za-z0-9-]*$
 * @maxLength 63
 */
export type Hostname = string;

/**
 * VRF, ACL, or policy-map name: letters, digits, "_", ".", and "-", at most 64 characters.
 * @pattern ^[A-Za-z0-9_.-]+$
 * @maxLength 64
 */
export type ObjectName = string;

/**
 * Free-form text (descriptions, pipe regexes): printable ASCII, not blank, and
 * without "|" or "?", which the CLI would interpret as a pipe or a help request.
 * Regex alternation ("a|b") is therefore not supported.
 * @pattern ^[ ->@-{}~]*[!->@-{}~][ ->@-{}~]*$
 */
export type CliText = string;

/**
 * VLAN ID, 1-4094.
 * @type integer
 * @minimum 1
 * @maximum 4094
 */
export type VlanId = number;

/**
 * VLAN list in IOS syntax, e.g. "10,20,30-40".
 * @pattern ^[0-9]+(-[0-9]+)?(,[0-9]+(-[0-9]+)?)*$
 */
export type VlanList = string;

// ---------------------------------------------------------------------------
// Document root
// ---------------------------------------------------------------------------

/** A full command block: an ordered list of top-level commands. */
export type CommandBlock = TopLevelCommand[];

export type TopLevelCommand =
  | ShowCommand
  | PingCommand
  | TracerouteCommand
  | CopyRunningConfigCommand
  | HostnameCommand
  | GlobalToggleCommand
  | InterfaceCommand;

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
  pipe?: OutputPipe;
}

/**
 * `show running-config [interface <name>]`
 * @title Show running config
 */
export interface ShowRunningConfig extends ShowBase {
  type: "show running-config";
  /** Restrict output to a single interface (`show running-config interface X`). */
  interface?: InterfaceName;
}

/**
 * `show startup-config`
 * @title Show startup config
 */
export interface ShowStartupConfig extends ShowBase {
  type: "show startup-config";
}

/**
 * `show interfaces [<name>] [status | description | switchport | trunk]`
 * @title Show interfaces
 */
export interface ShowInterfaces extends ShowBase {
  type: "show interfaces";
  interface?: InterfaceName;
  detail?: "status" | "description" | "switchport" | "trunk";
}

/**
 * `show interfaces [<name>] counters [errors | etherchannel | protocol status | trunk]`
 * @title Show interface counters
 */
export interface ShowInterfacesCounters extends ShowBase {
  type: "show interfaces counters";
  interface?: InterfaceName;
  detail?: "errors" | "etherchannel" | "protocol status" | "trunk";
}

/**
 * `show ip interface [brief | <name>]` (brief and interface are mutually exclusive)
 * @title Show IP interfaces
 */
export type ShowIpInterface = ShowBase & { type: "show ip interface" } & (
    | { brief?: boolean; interface?: never }
    | { brief?: false; interface?: InterfaceName }
  );

/**
 * `show ip route [vrf <vrf>] [<prefix>]`
 * @title Show IP routes
 */
export interface ShowIpRoute extends ShowBase {
  type: "show ip route";
  vrf?: ObjectName;
  prefix?: IPv4Address;
}

/**
 * `show ip arp [vrf <vrf>] [<addr> | <interface>]` (addr and interface are mutually exclusive)
 * @title Show ARP table
 */
export type ShowIpArp = ShowBase & { type: "show ip arp"; vrf?: ObjectName } & (
    | { addr?: IPv4Address; interface?: never }
    | { addr?: never; interface?: InterfaceName }
  );

/**
 * `show vlan [brief | id <vlan>]` (brief and id are mutually exclusive)
 * @title Show VLANs
 */
export type ShowVlan = ShowBase & { type: "show vlan" } & (
    | { brief?: boolean; id?: never }
    | { brief?: false; id?: VlanId }
  );

/**
 * `show cdp neighbors [detail]`
 * @title Show CDP neighbors
 */
export interface ShowCdpNeighbors extends ShowBase {
  type: "show cdp neighbors";
  detail?: boolean;
}

/**
 * `show lldp neighbors [detail]`
 * @title Show LLDP neighbors
 */
export interface ShowLldpNeighbors extends ShowBase {
  type: "show lldp neighbors";
  detail?: boolean;
}

/**
 * `show mac address-table [interface <name> | vlan <id>]` (interface and vlan are mutually exclusive)
 * @title Show MAC address table
 */
export type ShowMacAddressTable = ShowBase & { type: "show mac address-table" } & (
    | { interface?: InterfaceName; vlan?: never }
    | { interface?: never; vlan?: VlanId }
  );

/**
 * `show version`
 * @title Show version
 */
export interface ShowVersion extends ShowBase {
  type: "show version";
}

/**
 * `show inventory`
 * @title Show inventory
 */
export interface ShowInventory extends ShowBase {
  type: "show inventory";
}

/**
 * `show clock`
 * @title Show clock
 */
export interface ShowClock extends ShowBase {
  type: "show clock";
}

/**
 * `show logging`
 * @title Show logs
 */
export interface ShowLogging extends ShowBase {
  type: "show logging";
}

/**
 * `show users`
 * @title Show users
 */
export interface ShowUsers extends ShowBase {
  type: "show users";
}

/**
 * `show boot`
 * @title Show boot settings
 */
export interface ShowBoot extends ShowBase {
  type: "show boot";
}

/**
 * `show dhcp lease`
 * @title Show DHCP lease
 */
export interface ShowDhcpLease extends ShowBase {
  type: "show dhcp lease";
}

export type ShowCommand =
  | ShowRunningConfig
  | ShowStartupConfig
  | ShowInterfaces
  | ShowInterfacesCounters
  | ShowIpInterface
  | ShowIpRoute
  | ShowIpArp
  | ShowVlan
  | ShowCdpNeighbors
  | ShowLldpNeighbors
  | ShowMacAddressTable
  | ShowVersion
  | ShowInventory
  | ShowClock
  | ShowLogging
  | ShowUsers
  | ShowBoot
  | ShowDhcpLease;

// ---------------------------------------------------------------------------
// Other exec commands
// ---------------------------------------------------------------------------

/**
 * `ping <dest>`
 * @title Ping
 */
export interface PingCommand {
  type: "ping";
  dest: IPv4Address;
}

/**
 * `traceroute <dest>`
 * @title Traceroute
 */
export interface TracerouteCommand {
  type: "traceroute";
  dest: IPv4Address;
}

/**
 * `copy running-config startup-config`: saves the running config to startup.
 * @title Save running config
 */
export interface CopyRunningConfigCommand {
  type: "copy running-config startup-config";
}

// ---------------------------------------------------------------------------
// Global configuration commands
// ---------------------------------------------------------------------------

/**
 * `hostname <name>`
 * @title Set hostname
 */
export interface HostnameCommand {
  type: "hostname";
  hostname: Hostname;
}

interface Toggle {
  /** `true` emits the command; `false` emits its `no` form. */
  value: boolean;
}

/**
 * `cdp run`
 * @title Enable/disable CDP globally
 */
export interface CdpRunCommand extends Toggle {
  type: "cdp run";
}

/**
 * `lldp run`
 * @title Enable/disable LLDP globally
 */
export interface LldpRunCommand extends Toggle {
  type: "lldp run";
}

export type GlobalToggleCommand = CdpRunCommand | LldpRunCommand;

/**
 * `interface <name>` followed by indented sub-commands.
 *
 * If `default_first` is true, `default interface <name>` is issued before
 * entering the interface, resetting it to factory defaults.
 * @title Configure interface
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
 * `shutdown`
 * @title Shut down/bring up interface
 */
export interface ShutdownCommand extends Toggle {
  type: "shutdown";
}

/**
 * `switchport`
 * @title Enable/disable switchport (layer 2) mode
 */
export interface SwitchportCommand extends Toggle {
  type: "switchport";
}

/**
 * `negotiation auto`
 * @title Enable/disable autonegotiation
 */
export interface NegotiationAutoCommand extends Toggle {
  type: "negotiation auto";
}

/**
 * `cdp enable`
 * @title Enable/disable CDP on interface
 */
export interface CdpEnableCommand extends Toggle {
  type: "cdp enable";
}

/**
 * `lldp transmit`
 * @title Enable/disable LLDP transmit
 */
export interface LldpTransmitCommand extends Toggle {
  type: "lldp transmit";
}

/**
 * `lldp receive`
 * @title Enable/disable LLDP receive
 */
export interface LldpReceiveCommand extends Toggle {
  type: "lldp receive";
}

/**
 * `spanning-tree portfast`
 * @title Enable/disable PortFast
 */
export interface SpanningTreePortfastCommand extends Toggle {
  type: "spanning-tree portfast";
}

/**
 * `spanning-tree bpduguard enable`
 * @title Enable/disable BPDU guard
 */
export interface SpanningTreeBpduguardCommand extends Toggle {
  type: "spanning-tree bpduguard enable";
}

/**
 * `ip redirects`
 * @title Enable/disable ICMP redirects
 */
export interface IpRedirectsCommand extends Toggle {
  type: "ip redirects";
}

/**
 * `ip proxy-arp`
 * @title Enable/disable proxy ARP
 */
export interface IpProxyArpCommand extends Toggle {
  type: "ip proxy-arp";
}

/**
 * `ipv6 enable`
 * @title Enable/disable IPv6
 */
export interface Ipv6EnableCommand extends Toggle {
  type: "ipv6 enable";
}

export type ToggleCommand =
  | ShutdownCommand
  | SwitchportCommand
  | NegotiationAutoCommand
  | CdpEnableCommand
  | LldpTransmitCommand
  | LldpReceiveCommand
  | SpanningTreePortfastCommand
  | SpanningTreeBpduguardCommand
  | IpRedirectsCommand
  | IpProxyArpCommand
  | Ipv6EnableCommand;

/**
 * `description <text>`; `value: null` emits `no description`.
 * @title Set/clear description
 */
export interface DescriptionCommand {
  type: "description";
  value: CliText | null;
}

/**
 * `ip address <addr> <mask> [secondary]`
 * @title Set IPv4 address
 */
export interface IpAddressCommand {
  type: "ip address";
  addr: IPv4Address;
  mask: IPv4Mask;
  secondary?: boolean;
}

/**
 * `ip address dhcp`
 * @title Use DHCP for IPv4 address
 */
export interface IpAddressDhcpCommand {
  type: "ip address dhcp";
}

/**
 * `no ip address` (removes all IPv4 addresses).
 * @title Remove IPv4 addresses
 */
export interface NoIpAddressCommand {
  type: "no ip address";
}

/**
 * `ipv6 address <prefix> [eui-64]` or `ipv6 address <addr> link-local`;
 * `remove` emits the `no` form. With `link-local`, `prefix` is a bare
 * fe80::/10 address with no prefix length, e.g. "fe80::1".
 * @title Add/remove IPv6 address
 */
export interface Ipv6AddressCommand {
  type: "ipv6 address";
  prefix: IPv6Prefix;
  modifier?: "link-local" | "eui-64";
  remove?: boolean;
}

/**
 * `ip helper-address <addr>`; `remove` emits the `no` form.
 * @title Add/remove DHCP relay address
 */
export interface IpHelperAddressCommand {
  type: "ip helper-address";
  addr: IPv4Address;
  remove?: boolean;
}

/**
 * `vrf forwarding <name>`; `value: null` emits `no vrf forwarding`.
 * @title Set/clear VRF
 */
export interface VrfForwardingCommand {
  type: "vrf forwarding";
  value: ObjectName | null;
}

/**
 * `switchport mode <mode>`
 * @title Set switchport mode
 */
export interface SwitchportModeCommand {
  type: "switchport mode";
  mode: "access" | "trunk" | "dynamic auto" | "dynamic desirable";
}

/**
 * `switchport access vlan <id>`; `value: null` emits the `no` form.
 * @title Set/clear access VLAN
 */
export interface SwitchportAccessVlanCommand {
  type: "switchport access vlan";
  value: VlanId | null;
}

/**
 * `switchport voice vlan <id>`; `value: null` emits the `no` form.
 * @title Set/clear voice VLAN
 */
export interface SwitchportVoiceVlanCommand {
  type: "switchport voice vlan";
  value: VlanId | null;
}

/**
 * `switchport trunk native vlan <id>`; `value: null` emits the `no` form.
 * @title Set/clear trunk native VLAN
 */
export interface SwitchportTrunkNativeVlanCommand {
  type: "switchport trunk native vlan";
  value: VlanId | null;
}

/**
 * `switchport trunk allowed vlan [add | remove | except] <list>` / `... all` / `... none`
 * @title Set trunk allowed VLANs
 */
export interface SwitchportTrunkAllowedVlanCommand {
  type: "switchport trunk allowed vlan";
  action?: "set" | "add" | "remove" | "except";
  vlans: VlanList | "all" | "none";
}

/**
 * `speed <value>`; `value: null` emits `no speed`.
 * @title Set/reset speed
 */
export interface SpeedCommand {
  type: "speed";
  value: "auto" | "10" | "100" | "1000" | "2500" | "5000" | "10000" | "25000" | "40000" | "100000" | null;
}

/**
 * `duplex <value>`; `value: null` emits `no duplex`.
 * @title Set/reset duplex
 */
export interface DuplexCommand {
  type: "duplex";
  value: "auto" | "full" | "half" | null;
}

/**
 * `mtu <bytes>`; `value: null` emits `no mtu`.
 * @title Set/reset MTU
 */
export interface MtuCommand {
  type: "mtu";
  /**
   * @minimum 64
   * @maximum 9216
   */
  value: number | null;
}

/**
 * `ip mtu <bytes>`; `value: null` emits `no ip mtu`.
 * @title Set/reset IP MTU
 */
export interface IpMtuCommand {
  type: "ip mtu";
  /**
   * @minimum 68
   * @maximum 9216
   */
  value: number | null;
}

/**
 * `channel-group <n> mode <mode>`; `value: null` emits `no channel-group`.
 * @title Set/clear channel group
 */
export interface ChannelGroupCommand {
  type: "channel-group";
  value: {
    /**
     * @type integer
     * @minimum 1
     */
    group: number;
    mode: "active" | "passive" | "on" | "auto" | "desirable";
  } | null;
}

/**
 * `ip access-group <acl> {in | out}`; `remove` emits the `no` form.
 * @title Apply/remove access list
 */
export interface IpAccessGroupCommand {
  type: "ip access-group";
  acl: ObjectName;
  direction: "in" | "out";
  remove?: boolean;
}

/**
 * `service-policy output <policy>`; `remove` emits the `no` form.
 * @title Apply/remove output service policy
 */
export interface ServicePolicyOutputCommand {
  type: "service-policy output";
  policy: ObjectName;
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
  | IpAccessGroupCommand
  | ServicePolicyOutputCommand;
