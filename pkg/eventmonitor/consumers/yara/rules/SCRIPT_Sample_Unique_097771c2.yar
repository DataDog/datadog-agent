rule SCRIPT_Sample_Unique_097771c2 {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "097771c2c9c67d1c245d8245abecaa7778933afd6cfbc0e8f4de4baa3d7c78ff.bat"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "6b8e56e2-ff84-4b05-acf6-9395d8cd55ad"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "428d6ed6d9a4c36748e804112874bfe3"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "(for /r \"%USERPROFILE%\\Documents\" %%f in (*.pdf) do echo %%f) >> \"%pdfList%\" 2>nul" ascii wide
        $b = "for /f %%i in ('type \"%pdfList%\" 2^>nul ^| find /c /v \"\"') do set count=%%i" ascii wide
    condition:
        all of them
}
