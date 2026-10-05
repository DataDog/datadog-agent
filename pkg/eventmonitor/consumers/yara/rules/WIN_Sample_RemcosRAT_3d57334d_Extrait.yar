rule WIN_Sample_RemcosRAT_3d57334d_Extrait
{
    meta:
        author = "Marjoriefort"
        description = "Detects RemcosRAT (pe, etat extrait)"
        date = "2026-09-21"
        reference_sha256 = "3d57334d08deff6d7ba473d4388a4e45abb59ee8ddf96a5be81bdc333330ef57"
        yarahub_uuid = "249fdcf0-cb50-409f-85c0-2616eef5e054"
        famille = "RemcosRAT"
        famille_source = "reputation"
        classe = "pe"
        etat = "extrait"
        confidence_suggeree = "low"
        source = "forge_miss M3 v1.9.17"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "d202087f6b56891459e5ebc791b3c804"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $s0 = "lstRecettes_SelectedIndexChanged" ascii
        $s1 = "timerCycle_Tick" ascii
        $s2 = "Render_Tallow_Wick" ascii
        $s3 = "btnChoix1_Click" ascii
        $s4 = "btnChoix2_Click" ascii
        $s5 = "btnChoix3_Click" ascii
        $s6 = "btnDormir1h_Click" ascii
        $s7 = "btnDormir4h_Click" ascii
        $s8 = "btnDormir8h_Click" ascii
        $s9 = "btnAmeliorerAbri_Click" ascii
        $s10 = "btnSauvegarder_Click" ascii
        $s11 = "btnCharger_Click" ascii
        $s12 = "btnFouiller_Click" ascii
        $s13 = "btnConsommer_Click" ascii
        $s14 = "btnContinuer_Click" ascii
        $s15 = "btnFabriquer_Click" ascii
        $s16 = "btnCampement_Click" ascii
        $s17 = "btnRetourDesert_Click" ascii
        $s18 = "btnGauche_MouseDown" ascii
        $s19 = "btnDroite_MouseDown" ascii
    condition:
        (uint16(0) == 0x5A4D or uint32(0) == 0x04034b50) and filesize < 50MB and 3 of them and 1 of ($s0, $s1, $s2, $s3, $s4)
}
