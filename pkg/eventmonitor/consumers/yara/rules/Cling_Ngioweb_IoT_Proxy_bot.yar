/*
    YARA rule - Cling / Ngioweb IoT bot (residential proxy, C2 disguised as STUN)
    Author: eFeSpain  |  2026-09-04

    Usage note: the operator ROTATES the sample hashes (recompilation + multi-architecture),
    so hash-based indicators are of little use. This rule leans on behavioural STRINGS,
    which stay stable across recompilations and architectures. File/memory detection
    (complements the network signature). ELF (Linux) binaries only.
*/

rule Cling_Ngioweb_IoT_Proxy_bot
{
    meta:
        description    = "Cling/Ngioweb IoT bot (residential proxy, C2 disguised as STUN)"
        author         = "eFeSpain"
        author_url     = "https://efespain.com"
        date           = "2026-09-04"
        reference      = "https://blog.efespain.com/capitulo-19/"
        family_page    = "https://blog.efespain.com/cling/"
        malware_family = "Ngioweb"
        tlp            = "clear"
        yarahub_uuid              = "6ed570ab-af37-4f3f-8938-e7a7e9c83497"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "55284cf454972c245a1dfee7da6d7d87"
        yarahub_reference_link    = "https://blog.efespain.com/capitulo-19/"

    strings:
        // Persistence artifact (hidden binary) - stable across recompilations/architectures
        $path1   = "/root/.cling"           ascii
        $path2   = "/usr/local/bin/.cling"  ascii
        // Distinctive User-Agent it uses against the TR-064 CVE (Deutsche Telekom)
        $ua      = "clingwashere"           ascii
        // Exact auto-start line the bot writes into /etc/inittab
        $persist = "::once:/root/.cling"    ascii
        // Self-propagation tags (one per exploit) - unique to this family
        $tag1    = "tr064.selfrep"          ascii
        $tag2    = "selfrep.realtek"        ascii
        $tag3    = "realtek.selfrep"        ascii
        $tag4    = "selfrep.jaws"           ascii
        $tag5    = "selfrep.tbk"            ascii
        $tag6    = "selfrep.linksys"        ascii
        $tag7    = "selfrep.blink"          ascii

    condition:
        // ELF magic: 7F 45 4C 46 - fixed on EVERY architecture (little- and big-endian)
        uint8(0) == 0x7F and uint8(1) == 0x45 and uint8(2) == 0x4C and uint8(3) == 0x46
        and (
            ($ua and 1 of ($path*))    // the UA + one .cling path   -> very reliable
            or $persist                // the exact inittab line      -> unique to the family
            or 2 of ($tag*)            // two or more selfrep tags    -> distinctive
        )
}

