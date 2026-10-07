rule PureHVNC_PureCoder
{
    meta:
        id = "SDbKS5b4mBzBMinKiIB6Fh"
        fingerprint = "9159e6c469cea8f7e0a4f51380628cccb44620b782c4d07ecb80068c0d32b58e"
        version = "1.0"
        date = "2026-03-31"
        modified = "2026-03-31"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "PureHVNC (PureCoder) -- hidden VNC RAT with ProtoBuf C2, PE injection, credential theft, TLS cert pinning"
        category = "MALWARE"
        malware = "PUREHVNC"
        mitre_att = "T1219"

        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "PureHVNC PureCoder hidden VNC RAT detected."
        yarahub_uuid = "990a9042-ede7-4cdd-ba52-35d132d301a5"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "e2759b5ef495bfcfad9074678497f649"

    strings:
        // .NET namespace (plaintext in metadata)
        $ns_purehvnc = "PureHVNC_Lib" ascii

        // Obfuscation attribute marker
        $obf_marker = "EZNRMERM" ascii

        // Assembly GUID
        $guid = "92342e74-3496-442e-8919-4ff580898524" ascii

        // Partially obfuscated namespace
        $ns_obf = "Lhjknyy" ascii wide

        // Canary/junk strings embedded by obfuscator (unique to this build)
        $canary1 = "MKIQFWNK1Zm4dU51L6W" ascii wide
        $canary2 = "IH9HVK9UpOrjBfljU" ascii wide

    condition:
        uint16(0) == 0x5A4D
        and filesize > 200KB and filesize < 2MB
        and (
            // Tier 1: namespace is definitive
            $ns_purehvnc
            or (
                // Tier 2: obfuscation marker + obfuscated namespace
                $obf_marker and $ns_obf
            )
            or (
                // Tier 3: GUID + at least one canary
                $guid and 1 of ($canary1, $canary2)
            )
        )
}
