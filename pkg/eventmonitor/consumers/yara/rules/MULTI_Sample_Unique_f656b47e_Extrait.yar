rule MULTI_Sample_Unique_f656b47e_Extrait
{
    meta:
        author = "Marjoriefort"
        description = "Detects Unknown (inconnu, etat extrait)"
        date = "2026-09-21"
        reference_sha256 = "f656b47e2337fea418a36933f173da98901d9ba4c0e05e3457ee3c59f94ad458"
        yarahub_uuid = "01d1dbc3-ff4c-4f37-a94f-fed8456fec69"
        famille = "unknown"
        famille_source = "inconnue"
        classe = "inconnu"
        etat = "extrait"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.17"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "d8029c435c533deebb11bb4fc7d2d780"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "almohtraf.exe" ascii
        $s1 = "schtasks /Delete /TN \"WinServiceSyncTask\" /F > nul 2>&1" ascii
        $s2 = "timeout /t 5 /nobreak > nul" ascii
        $s3 = "if exist \"almohtraf.exe\" (" ascii
        $s4 = "start \"\" /b \"almohtraf.exe\"" ascii
        $s5 = "if exist \"..\\update_pkg.dat\" (" ascii
        $s6 = "del /f /q \"..\\update_pkg.dat\" > nul 2>&1" ascii
        $s7 = "start /b" ascii
        $s8 = "WinServiceSyncTask" ascii
        $s9 = "..\\update_pkg.dat" ascii
        $s10 = "AppData)" ascii
        $s11 = "Background)" ascii
    condition:
        true and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
