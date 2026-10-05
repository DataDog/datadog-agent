rule SCRIPT_Sample_Unique_6413f868 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "6413f8681087402683d64074a7ff58d4f555566edb7a404634190c92b071a080.bat"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "0d010635-6da0-415f-b153-e0a28144335d"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "0f906a7d2fc2b0bb73edcad8bc45bbdb"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "netsh advfirewall firewall add rule name=\"Intel Remote Management\" dir=in action=allow program=\"%TargetDir%\\rserver3.exe\" enable=yes profile=any >nul 2>&1" ascii wide
        $b = "sc failure RServer3 reset= 0 actions= restart/3000/restart/3000/restart/3000 >nul 2>&1" ascii wide
    condition:
        all of them
}
