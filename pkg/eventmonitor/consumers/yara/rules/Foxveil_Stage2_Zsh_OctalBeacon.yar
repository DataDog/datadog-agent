rule Foxveil_Stage2_Zsh_OctalBeacon
{
    meta:
        yarahub_uuid = "010b0e37-5ef3-41f2-828a-d0dd8a61ca8a"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "522c30270d899f2059951c0b468b1f7a"
        description = "Foxveil ClickFix stage-2 zsh: octal-printf strings, /api/metrics/run?event= beacon with user and BuildID headers, payload fetched to /tmp and run"
        actor = "Foxveil"
        family = "AMOS ClickFix dropper"
        reference_sha256 = "55b8fff6b61ed493f477bac718c37714075d4ff52bc302d28a331def371834b6"
        date = "2026-09-18"
        tlp = "CLEAR"

    strings:
        $shebang  = "#!/bin/zsh"
        // octal for "/api/metrics/run?event="
        $beacon   = "\\057\\141\\160\\151\\057\\155\\145\\164\\162\\151\\143\\163\\057\\162\\165\\156\\077\\145\\166\\145\\156\\164\\075"
        // octal for "https://"
        $https    = "\\150\\164\\164\\160\\163\\072\\057\\057"
        // octal for "/update" closing a printf string
        $update   = "\\057\\165\\160\\144\\141\\164\\145')"
        $h_user   = "-H 'user: Ag"
        $h_build  = "-H 'BuildID: Ag"
        $quiet    = "</dev/null >/dev/null 2>&1 &"

    condition:
        filesize < 16KB and $shebang at 0 and $beacon and #https >= 2 and
        $h_user and $h_build and ($update or $quiet)
}
