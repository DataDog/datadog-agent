rule EDRChoker_Killer_WMI_QoS_Throttle : Hacktool
{
    meta:
        name                        = "EDRChoker_Killer_WMI_QoS_Throttle"
        category                    = "Hacktool"
        description                 = "EDRChoker - .NET hacktool that abuses WMI NetQoS policies to throttle/bandwidth-limit EDR/security process traffic"
        author                      = "Valton Tahiri"
        created                     = "2026-06-08"
        date                        = "2026-06-08"
        tlp                         = "TLP:white"
        reliability                 = 90
        reference                   = "https://x.com/TwoSevenOneT"
        imphash                     = "f34d5f2d4577ed6d9ceec516c1f5a744"
        sample                      = "c851656ded75a7fe2e1035b9ea54ae7c4d2cdfdd79cafc462f9ac3e8ba845cef"
        yarahub_uuid                = "b3c2dda6-a914-4182-988b-f886050bcabe"
        yarahub_license             = "CC0 1.0"
        yarahub_rule_matching_tlp   = "TLP:WHITE"
        yarahub_rule_sharing_tlp    = "TLP:WHITE"
        yarahub_reference_md5       = "1b239a41d8c2ba5d32adaec13ce20b10"

    strings:
        // --- High-confidence unique identifiers ---
        $s_tagline      = "EDRChoker: You can pass, but just a little" wide
        $s_author       = "Two Seven One Three: x.com/TwoSevenOneT" wide
        $s_create_wmi   = "CreateThrottleCurlPolicyPureWmi" ascii wide
        $s_remove_wmi   = "RemoveAllThrottleCurlPoliciesPureWmi" ascii wide
        $s_throttle_log = "THROTTLING! Process: " wide
        $s_do_clear     = "DO CLEAR..." wide
        $s_pdb_path     = "bandwith_limit\\C#\\EDRChoker\\obj\\Release\\EDRChoker.pdb" ascii wide nocase
        $s_wmi_query    = "SELECT * FROM MSFT_NetQosPolicySettingData" wide

        // --- WMI / NetQoS abuse surface ---
        $s_wmi_ns       = "\\\\.\\ROOT\\StandardCimv2" wide
        $s_qos_setting  = "MSFT_NetQosPolicySettingData" ascii wide
        $s_app_match    = "AppPathNameMatchCondition" wide
        $s_ip_match     = "IPProtocolMatchCondition" wide
        $s_throttle_act = "ThrottleRateAction" wide
        $s_success_pol  = "SUCCESS! Policy " wide

        // --- .NET / PE metadata ---
        $s_dotnet_rt    = ".NETFramework,Version=v4.8" ascii wide
        $s_mscoree      = "_CorExeMain" ascii

        // --- Binary / hex patterns ---
        $h_dotnet_bsjb  = { 42 53 4A 42 }
        $h_pdb_utf8     = { 62 61 6E 64 77 69 74 68 5F 6C 69 6D 69 74 5C 43 23 5C 45 44 52 43 68 6F 6B 65 72 5C 6F 62 6A 5C 52 65 6C 65 61 73 65 5C 45 44 52 43 68 6F 6B 65 72 2E 70 64 62 }

        // IL: CreateThrottleCurlPolicyPureWmi (newarr + constrained. prefix)
        $h_il_create = { 17 8C 2B 00 00 01 6F [4] 28 [4] 13 04 12 04 FE 16 17 00 00 01 6F [4] 0B 28 }

        // IL: RemoveAllThrottleCurlPoliciesPureWmi (leave.s + enumerator loop)
        $h_il_remove = { DD DA 00 00 00 08 6F [4] 0A 0D 2B 54 09 6F [4] 0A 74 [4] 25 72 [4] 6F [4] 0A }

        // IL: ShowBanner (ConsoleColor cycling ASCII art)
        $h_il_banner = { 1F 0B 28 [4] 72 [4] 28 [4] 72 [4] 28 [4] 72 [4] 28 [4] 1D 28 [4] 1F 0F 28 [4] 2A }

        // IL: Main per-process throttle loop (admin gate + List iteration)
        $h_il_main   = { 02 8E 2C 4D 06 02 16 9A 6F [4] 06 06 7B [4] 04 8E 2D 0B 72 [4] 28 [4] 2A }

    condition:
        uint16(0) == 0x5A4D and
        uint32(uint32(0x3C)) == 0x00004550 and
        filesize < 1MB and
        (
            // Tier 1: unique WMI method pair (name or IL)
            ( $s_create_wmi and $s_remove_wmi ) or
            ( $h_il_create and $h_il_remove ) or

            // Tier 2: IL create + QoS WMI query chain
            (
                $h_il_create and
                2 of ( $s_wmi_query, $s_qos_setting, $s_app_match, $s_ip_match, $s_throttle_act, $s_wmi_ns )
            ) or

            // Tier 3: tagline + banner IL + behavioral strings
            (
                $s_tagline and
                ( $h_il_banner or $s_author ) and
                2 of ( $s_throttle_log, $s_do_clear, $s_success_pol, $s_wmi_query )
            ) or

            // Tier 4: dev PDB + .NET markers + WMI logic
            (
                ( $s_pdb_path or $h_pdb_utf8 ) and
                $h_dotnet_bsjb and
                $s_dotnet_rt and
                1 of ( $h_il_create, $h_il_remove, $s_create_wmi, $s_remove_wmi )
            ) or

            // Tier 5: stripped strings - IL-only fallback
            (
                $h_il_main and
                $h_il_create and
                $h_dotnet_bsjb and
                $s_mscoree
            )
        )
}
