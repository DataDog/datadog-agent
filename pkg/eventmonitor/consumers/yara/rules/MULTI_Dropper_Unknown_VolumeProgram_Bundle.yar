rule MULTI_Dropper_Unknown_VolumeProgram_Bundle {
    meta:
        description = "Archive avec membre 'Volume/program' - bundle dropper multi-plateforme (16 soumissions identiques)"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "misses_archive / grappe 16 archives"
        confidence  = "medium-high"
        yarahub_uuid            = "490bb8f2-3390-42f6-a1a0-71f91e442993"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "1adb634b0b498a770a19f53788ca4039"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $zip = { 50 4B 03 04 }
        $a   = "Volume/program" ascii wide
    condition:
        $zip and $a
}
