rule SCRIPT_Sample_Unique_7f3f3e93 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "7f3f3e9333195ccca7b017a842df754db5fc13d4b48e9f849cb5b324258f4d83.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "4b3b3a9d-6210-4921-b128-63fedc20dd3e"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "629266ab96e04a606cc73aff01bb640c"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "Write-Host \"  Scanned: $pagesScanned pages ($([math]::Round($pagesScanned*$pageSize/1KB)) KB of $([math]::Round($size/1MB,1)) MB)\"" ascii wide
        $b = "                            Write-Host \"  JWT: $($jwt.Substring(0, [math]::Min(80,$jwt.Length)))...\"" ascii wide
    condition:
        all of them
}
