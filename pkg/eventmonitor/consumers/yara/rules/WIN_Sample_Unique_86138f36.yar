rule WIN_Sample_Unique_86138f36 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "86138f3695b0bc0f0dc446233b3d9b9bba2260a81f56c66bc870646da40ad1da.exe"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "332926b5-2d73-49a4-970b-eaed6286bf4b"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "a1849f07de8f4b7a89683ca7284ba9a1"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "LCleanroomConnectLauncher.LauncherForm+<InstallPrivateAccessClientAsync>d__26" ascii wide
        $b = "        <requestedExecutionLevel level=\"asInvoker\" uiAccess=\"false\"/>" ascii wide
    condition:
        all of them
}
