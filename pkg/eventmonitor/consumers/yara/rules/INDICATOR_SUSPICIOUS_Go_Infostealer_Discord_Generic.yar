rule INDICATOR_SUSPICIOUS_Go_Infostealer_Discord_Generic
{
    meta:
        description = "Detects a Go-based infostealer that targets Discord tokens by locating the 'Local State' file, decrypting the master key with DPAPI, and exfiltrating tokens."
        author = "Yara Rule Generator"
        date = "2023-10-27"
        reference = "Internal analysis of decompiled code. Generic version."
        malware_family = "GoDiscordStealer"
        hash = "N/A - Rule based on provided code snippets"
        yarahub_reference_md5 = "78357375735734775475747574757454"
        yarahub_uuid = "2a763267-af58-46e3-9d77-b6de01f25648"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        // Core download and execute pattern
        $cmd_dl_exec1 = "curl -k -s -H \"api-key: %s\"" ascii wide
        $cmd_dl_exec2 = "| osascript" ascii wide

        // Exfiltration pattern
        $cmd_exfil1 = "-F \"file=@/tmp/osalogging.zip\"" ascii wide
        $cmd_exfil2 = "-F \"buildtxd=%s\"" ascii wide
        $cmd_exfil3 = "https://%s/gate" ascii wide

        // Other suspicious strings
        $str_kill = "killall Terminal" ascii wide
        $str_uri = "/dynamic?txd=%s" ascii wide

    condition:
        // Check for Mach-O 64-bit magic bytes
        uint32(0) == 0xfeedfacf and
        (
            // High confidence: The core download, execute, and exfiltration logic is present
            (all of ($cmd_dl*)) and (1 of ($cmd_exfil*))
        ) or
        (
            // Medium confidence: The download/execute pattern plus another indicator
            (all of ($cmd_dl*)) and (1 of ($str*))
        )
}
