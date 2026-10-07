rule SCRIPT_Sample_Unique_2d8d1751 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "2d8d17516b60bd316236db1bdf6b6ae6084d32d4146414810433a326d75b23f8.js"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "61ba4c71-e87c-404f-b331-9df5b3a535dc"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "46ee536a83379da6bfe4841a38b663bf"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "                      var e = new Enumerator(service.ExecQuery(\"Select * from Win32_LogicalProgramGroupItemDataFile\", null, 48));" ascii wide
        $b = "                      var e = new Enumerator(service.ExecQuery(\"Select * from Win32_LogicalProgramGroupDirectory\", null, 48));" ascii wide
    condition:
        all of them
}
