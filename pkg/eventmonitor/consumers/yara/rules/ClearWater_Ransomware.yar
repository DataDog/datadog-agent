rule ClearWater_Ransomware
{
    meta:
        author                      = "Arrbat"
        date                        = "2026-05-28"
        family                      = "win.clearwater"
        description                 = "Detects ClearWater ransomware using a mix of family-specific names, 7-Zip SFX metadata, and anti-recovery indicators"
        yarahub_author_github       = "Arrbat"
        yarahub_reference_md5       = "cf4840ae85d7acba4974d6dd55893d6c"
        yarahub_uuid                = "4ecbb03d-7d36-4995-973d-62ffb2a8b267"
        yarahub_license             = "CC0 1.0"
        yarahub_rule_matching_tlp   = "TLP:WHITE"
        yarahub_rule_sharing_tlp    = "TLP:WHITE"

    strings:
        $id1 = "ClearWater_x64.exe"             ascii wide nocase
        $id2 = "CLEARWATER_README.txt"          ascii wide nocase
        $id3 = "ClearWater Ransomware Starting" ascii wide nocase
        $id4 = ".clear"                         ascii wide nocase
        $id5 = "README.bmp.clear"               ascii wide nocase

        $sfx1 = "7ZSfxMod_x64.exe"              ascii wide nocase
        $sfx2 = "7ZSfxMod"                      ascii wide nocase
        $sfx3 = "Oleg N. Scherbakov"            ascii wide nocase
        $sfx4 = "RequireAdministrator=\"yes\""  ascii wide nocase
        $sfx5 = "SelfDelete=\"1\""              ascii wide nocase
        $sfx6 = "C:\\ProgramData"               ascii wide nocase

        $rn1 = ".clear"                         ascii wide nocase
        $rn2 = "decrypt"                        ascii wide nocase
        $rn3 = "encrypted"                      ascii wide nocase
        $rn4 = "recover your files"             ascii wide nocase
        $rn5 = "ransom"                         ascii wide nocase

    condition:
        uint16(0) == 0x5A4D and
        filesize < 20MB     and
        (
            (
                1 of ($id*)  and
                1 of ($sfx*) and
                2 of ($rn*) 
            )
        )
}