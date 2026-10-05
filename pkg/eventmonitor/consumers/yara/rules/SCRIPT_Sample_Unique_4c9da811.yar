rule SCRIPT_Sample_Unique_4c9da811 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "4c9da8110936aa6e8d1c1174b34032520e7f794f1270a7a61f71fb025084e7bb.unknown"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "7ac45dff-d603-4d92-85ec-f03b03e63ade"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "7e13cbd8199e4d34d22b54f35cfcf8c8"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "Remove-Item $MyInvocation.MyCommand.Path -Force -ErrorAction SilentlyContinue" ascii wide
        $b = "[IO.File]::WriteAllText(\"$d\\cursor_access_token.txt\",$m.Groups[1].Value)" ascii wide
    condition:
        all of them
}
