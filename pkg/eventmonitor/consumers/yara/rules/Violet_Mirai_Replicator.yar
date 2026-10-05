rule Violet_Mirai_Replicator {
    meta:
        description = "Violet Mirai variant with URLhaus Replicator module"
        author = "Nokia Deepfield ERT"
        date = "2026-03-08"
        modified = "2026-03-27"
        family = "Violet"
        hash = "54df22fd90c45f9e5969ee574aeb4ca6c7aacd394309d7e919d8d3655c61dd38"
        yarahub_uuid = "cd3527e4-a169-4821-9406-35f5a7aa5e03"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "ef49779bce7ff97d4613d48f153d8d65"

    strings:
        // Replicator module log strings
        $repl1 = "[Replicator] Starting local scan for telnet targets..."
        $repl2 = "[Replicator] Local scan complete. Now fetching from URLhaus..."
        $repl3 = "[Replicator] Found %d IPs to attempt"
        $repl4 = "[Replicator] Successfully infected %d.%d.%d.%d!"
        $repl5 = "[Replicator] No IPs fetched, retrying in 5 minutes"

        // Binary payload names
        $bin1 = "violetarm"
        $bin2 = "violetmips"
        $bin3 = "violetmpsl"
        $bin4 = "violetx86"
        $bin5 = "violetppc"
        $bin6 = "violetspc"

        // URLhaus integration
        $url1 = "urlhaus-api.abuse.ch"
        $url2 = "/downloads/csv_recent/"

        // Anti-analysis decoy strings
        $decoy1 = "[ERROR] Failed to initialize network subsystem (err=10048)"
        $decoy2 = "[FATAL] Memory allocation failed at 0x00402010"
        $decoy3 = "[CRITICAL] Watchdog timeout detected, shutting down"

        // Anti-debug tool names
        $dbg1 = "libfrida"
        $dbg2 = "radare2"

        // Process management
        $proc1 = "pkill -9 scanner 2>/dev/null"
        $proc2 = "pkill -9 replicator 2>/dev/null"

    condition:
        uint32(0) == 0x464C457F and (
            // Strong: Replicator strings
            2 of ($repl*) or
            // Medium: violet binary names + URLhaus
            (2 of ($bin*) and 1 of ($url*)) or
            // Broad: URLhaus + decoys + anti-debug
            (1 of ($url*) and 1 of ($decoy*) and 1 of ($dbg*)) or
            // Process management combo
            ($proc1 and $proc2 and 1 of ($bin*))
        )
}
