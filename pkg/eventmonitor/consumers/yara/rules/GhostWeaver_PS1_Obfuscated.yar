rule GhostWeaver_PS1_Obfuscated
{
    meta:
        id = "4yK2n9g8FzSWbGDNR8CDEB"
        fingerprint = "3be5de8be19b208ce66729829aec63a8e5b94d897bcfa15618d191013d22e3d4"
        version = "1.0"
        date = "2026-03-08"
        modified = "2026-03-08"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://WWW.DERP.CA/BLOG/GHOSTWEAVER-TAG124-POWERSHELL-RAT"
        author = "derp.ca"
        description = "GhostWeaver/Pantera RAT - obfuscated PowerShell builder output"
        category = "MALWARE"
        malware = "GHOSTWEAVER"
        mitre_att = "T1059.001"
        reference = "https://www.derp.ca/blog/ghostweaver-tag124-powershell-rat"
        triage_score = 10
        triage_description = "GhostWeaver/Pantera RAT obfuscated PowerShell builder output detected."
        yarahub_uuid = "3136440f-87b5-401b-b709-7cf3691b5e35"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "78113996c94c5a1fcacba8da4ca86dde"

    strings:
        $builder_stamp = /\$global:keystr=streams\\[0-9]{3}\\stub\\[0-9]{8,15}/ ascii wide

        $arith_decoder1 = "[system.String]::new(@((" ascii wide nocase
        $arith_decoder2 = "[char[]]@((" ascii wide nocase

        $arith_sep = "),(" ascii wide

        $exec_ctx = "$executioncontext" ascii wide nocase

        $join_op = "-join ''" ascii wide

    condition:
        filesize > 200KB and filesize < 1MB
        and (
            $builder_stamp
            or (
                ($arith_decoder1 or $arith_decoder2)
                and #arith_sep > 500
                and $exec_ctx
                and $join_op
            )
        )
}
