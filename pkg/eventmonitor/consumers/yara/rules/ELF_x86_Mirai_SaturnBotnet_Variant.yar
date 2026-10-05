rule ELF_x86_Mirai_SaturnBotnet_Variant {
    meta:
        description = "Detects Mirai-derived SaturnBotnet variant targeting Linux x86 devices via Huawei, GPON, and JAWS RCE exploits"
        author = "Serhii Kocherhan"
        date = "2026-09-02"
        yarahub_author_twitter = "@skocherhan"
        yarahub_uuid = "2d8f1594-c263-4615-9d5e-d1a9399d6372"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "11ac35f8ecd526e9160313a516e540ac"

    strings:
        // Linux ELF Magic Bytes
        $elf_magic = { 7F 45 4C 46 }

        // Specific Signature String & C2 Domain
        $sig_str    = "unstable_is_the_history_of_universe" ascii wide
        $c2_domain  = "scan.saturnbotnet" ascii wide nocase

        // Payload Download Request URL
        $jaws_payload = "http://127.0.0.1/shell?cd+/tmp;rm+-rf" ascii wide

        // Specific Device Exploit URIs
        $uri_huawei = "/ctrlt/DeviceUpgrade_1" ascii
        $uri_gpon   = "/GponForm/diag_Form" ascii
        $uri_jaws   = "/shell" ascii

        // Telnet / System Inspection Signatures
        $telnet_prompt = "pA5sW0rd" ascii wide
        $proc_mounts   = "/proc/mounts" ascii

    condition:
        // Ensure standard 32-bit x86 ELF header (EM_386 = 0x0003)
        $elf_magic at 0 and uint16(0x12) == 0x0003 and
        filesize < 5MB and
        (
            // Primary matches: High-confidence unique strings
            $sig_str or
            $c2_domain or
            $jaws_payload or
            
            // Secondary match: Combination of exploit URIs and process/telnet capabilities
            (
                2 of ($uri_*) and
                1 of ($telnet_prompt, $proc_mounts)
            )
        )
}