rule ECHidna_Loader
{
    meta:
        id = "2Z3cMJ7JbX5NRIoKRSUiHq"
        fingerprint = "d2ecd2e99342e7e03a7fc45fd853e4fb1fbb1aa74b3228f3ff2116af15a82ca8"
        version = "1.0"
        date = "2026-03-09"
        modified = "2026-03-09"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "ECHidna BoringSSL wrapper DLL with compressed RAT payload in PE overlay."
        category = "MALWARE"
        malware = "ECHIDNA"
        mitre_att = "T1027.009"
        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "ECHidna BoringSSL loader with embedded payload detected."
        yarahub_uuid = "ab4a45d9-1be1-4571-8564-e05c287e58c6"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "5321d27958a11f9744cf23be0204f386"

    strings:
        // Overlay markers followed by reflective loader shellcode (push 0, call, ret, int3 padding)
        $overlay_bfbf = { BF BF BF BF 6A 00 E8 09 00 00 00 C3 CC CC CC CC }
        $overlay_dadd = { BF AD DD DA 6A 00 E8 09 00 00 00 C3 CC CC CC CC }

        // Build path unique to WebRCS project (in BoringSSL assert messages)
        $build_webrcs = "boringssl_x86\\build\\WebRCS" ascii

        // C2 path survives LZ77 compression in overlay
        $c2_path = "/webrcs/" ascii

        // Host ID in compressed overlay
        $host_id = /[0-9]{4,5}ECH\(x86\)/ ascii

        // SSL_version export used as DLL entry point
        $export_ssl = "SSL_version" ascii

    condition:
        uint16(0) == 0x5a4d
        and filesize > 1MB and filesize < 3MB
        and (
            ($overlay_bfbf or $overlay_dadd)
            or ($build_webrcs and ($c2_path or $host_id))
            or ($export_ssl and $c2_path and $build_webrcs)
        )
}
