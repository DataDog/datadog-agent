rule Web_Phishing_ClickFix_AntiAnalysis_Script {
    meta:
        description = "Detects ClickFix social engineering script payloads using environment checks (isHeadless, isLocalhost) and anti-analysis console messages."
        author = "Serhii Kocherhan"
        date = "2026-09-19"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "0858aab6-6169-400e-9b8e-b159d6590090"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://x.com/inf0stache/status/2101426244927578536"
        yarahub_reference_md5 = "00000000000000000000000000000000"

    strings:
        // Unique string artifact from the anti-analysis block
        $s_msg = "stop watching us :)" ascii wide

        // Localhost checking patterns
        $s_localhost1 = "\"localhost\" === t" ascii wide
        $s_localhost2 = "\"127.0.0.1\" === t" ascii wide
        $s_localhost3 = "t.endsWith(\".localhost\")" ascii wide

        // Platform detection check
        $s_win = "isWindows = navigator.userAgent.includes(\"Windows\")" ascii wide
        $s_mac = "isMac = navigator.userAgent.includes(\"Macintosh\")" ascii wide

    condition:
        filesize < 100KB and (
            $s_msg or
            (any of ($s_localhost*) and ($s_win or $s_mac))
        )
}