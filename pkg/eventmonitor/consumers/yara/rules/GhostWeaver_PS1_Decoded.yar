rule GhostWeaver_PS1_Decoded
{
    meta:
        id = "4gcK8OxIKebmDMhHw8lPiz"
        fingerprint = "e5c29d0b3ca8651ba6c688f3ec5d0c8f9645ea55aeb17956a32717179cc9fb3a"
        version = "1.0"
        date = "2026-03-08"
        modified = "2026-03-08"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://WWW.DERP.CA/BLOG/GHOSTWEAVER-TAG124-POWERSHELL-RAT"
        author = "derp.ca"
        description = "GhostWeaver/Pantera RAT - decoded PowerShell (memory or post-deobfuscation)"
        category = "MALWARE"
        malware = "GHOSTWEAVER"
        mitre_att = "T1059.001"
        reference = "https://www.derp.ca/blog/ghostweaver-tag124-powershell-rat"
        triage_score = 10
        triage_description = "GhostWeaver/Pantera RAT decoded PowerShell payload detected."
        yarahub_uuid = "d4d68ed7-7aad-4296-bfe6-744b3c5f3301"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "78113996c94c5a1fcacba8da4ca86dde"

    strings:
        $proto_ssl = "SslStream" ascii wide nocase
        $proto_gzip = "GzipStream" ascii wide nocase
        $proto_json_in = "ConvertFrom-Json" ascii wide nocase
        $proto_json_out = "ConvertTo-Json" ascii wide nocase
        $proto_sslproto = "SslProtocols" ascii wide nocase

        $beacon = "'ClientInfo'" ascii wide
        $cmd_iex = "'iex'" ascii wide
        $cmd_plugin = "'plugin'" ascii wide
        $cmd_selfdelete = "'selfdelete'" ascii wide
        $cmd_saveplugin = "'savePlugin'" ascii wide
        $cmd_sendplugin = "'sendPlugin'" ascii wide

        $mutex = "euzizvuze" ascii wide

        $port = "25658" ascii wide

        $dga_random = "System.Random" ascii wide nocase
        $dga_doy = "DayOfYear" ascii wide nocase
        $dga_tld_top = ".top" ascii wide
        $dga_tld_fun = ".fun" ascii wide
        $dga_tld_xyz = ".xyz" ascii wide
        $dga_tld_cn = ".cn" ascii wide

    condition:
        (2 of ($proto_*))
        and (
            ($beacon and 3 of ($cmd_*))
            or
            ($mutex and $port and $dga_random and $dga_doy and 3 of ($dga_tld_*))
            or
            (4 of ($cmd_*) and $mutex)
        )
}
