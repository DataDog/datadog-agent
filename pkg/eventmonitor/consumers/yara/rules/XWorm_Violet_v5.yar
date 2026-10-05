rule XWorm_Violet_v5
{
    meta:
        id = "7AXQEJIq4084eHO9b1OQcK"
        fingerprint = "6d279e229932392323fef344c12c921620e5e7db573d3b3b1f00861ff55e2e1b"
        version = "1.0"
        date = "2026-03-31"
        modified = "2026-03-31"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "XWorm / Violet v5 -- 80+ command RAT with HVNC, keylogger, crypto clipper, USB worm, webcam capture"
        category = "MALWARE"
        malware = "XWORM"
        mitre_att = "T1219"

        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "XWorm Violet v5 RAT detected."
        yarahub_uuid = "74ba7289-0235-4623-bae1-1a96abe95c18"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "403f1a3b591c6da42efd290ec3094cdd"

    strings:
        // Drop filename stored as base64 in binary (UTF-16LE)
        // WinSc32.exe
        $drop_name = "WinSc32" ascii wide

        // Cleanup bat stored as base64 -> decoded, also present as UTF-16LE
        // WinTempClean32.bat -> base64 V2luVGVtcENsZWFuMzIuYmF0
        $cleanup_b64 = "V2luVGVtcENsZWFuMzIuYmF0" ascii wide

        // Webcam capture via avicap32.dll
        $webcam_api = "capCreateCaptureWindowA" ascii

        // HVNC desktop switching
        $hvnc_api = "OpenInputDesktop" ascii

        // Version string base64: Violet v5 -> VmlvbGV0IHY1
        $version_b64 = "VmlvbGV0IHY1" ascii wide

        // C2 protocol separator base64: XSXSXSX -> WFNYU1hTWA==
        $sep_b64 = "WFNYU1hTWA" ascii wide

        // Base64 of mutex HOHE6S8FaZZlGf0f -> SE9IRTZTOEZhWlpsR2YwZg==
        $mutex_b64 = "SE9IRTZTOEZhWlpsR2YwZg" ascii wide

        // Update screen commands (base64 encoded)
        // ShowUpdateScreen -> U2hvd1VwZGF0ZVNjcmVlbg==
        $show_update_b64 = "U2hvd1VwZGF0ZVNjcmVlbg" ascii wide
        // HideUpdateScreen -> SGlkZVVwZGF0ZVNjcmVlbg==
        $hide_update_b64 = "SGlkZVVwZGF0ZVNjcmVlbg" ascii wide

        // SERPENTINE#CLOUD cross-campaign GUID (same in 02192026 and 03312026)
        // 3d847c5c-4f5a-4918-9e07-a96cea49048d -> base64 M2Q4NDdjNWMtNGY1YS00OTE4LTllMDctYTk2Y2VhNDkwNDhk
        $guid_b64 = "M2Q4NDdjNWMtNGY1YS00OTE4LTllMDctYTk2Y2VhNDkwNDhk" ascii wide

    condition:
        uint16(0) == 0x5A4D
        and filesize > 30KB and filesize < 500KB
        and (
            // Tier 1: cross-campaign GUID (unique to this operator)
            $guid_b64
            or (
                // Tier 2: Violet version + protocol separator
                $version_b64 and $sep_b64
            )
            or (
                // Tier 3: behavioral combination -- drop artifacts + HVNC + webcam
                ($drop_name or $cleanup_b64)
                and ($webcam_api or $hvnc_api)
                and 2 of ($show_update_b64, $hide_update_b64, $mutex_b64, $sep_b64)
            )
        )
}
