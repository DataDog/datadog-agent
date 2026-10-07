import "elf"

rule elf_airashi_2025_strings_yarahub
{
  meta:
    author = "anonymous"
    date = "2025-08-23"
    family = "AIRASHI/AISURU"
    description = "MIPS BE ELF IoT botnet; matches AIRASHI/kitty markers"
    yarahub_uuid = "2e0a8715-b595-4f87-9857-01c3d57a6c94"
    yarahub_license = "CC0 1.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
    yarahub_reference_md5 = "939c3a8d56cf4f8aec415842d28c6cae"

  strings:
    $s1 = "AIRASHI: applet not found"
    $s2 = "/bin/busybox AIRASHI"
    $s3 = "Kitty-Kitty-Kitty"
    $s4 = "meow!"
    $s5 = "stun.l.google.com:19302"
    $s6 = "/proc/cpuinfo"

  condition:
    uint32(0) == 0x7F454C46 and       // ELF
    uint8(4) == 1 and                 // 32-bit
    uint8(5) == 2 and                 // big-endian
    elf.machine == 8 and              // EM_MIPS
    2 of ($s*)
}
