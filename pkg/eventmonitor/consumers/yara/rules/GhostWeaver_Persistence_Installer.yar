rule GhostWeaver_Persistence_Installer
{
    meta:
        id = "5JZRC0pj4MOxL8jRH8TIdt"
        fingerprint = "c419a47785d9adc837c684d133891799f8e300fe2d46195e28553abcdf74289c"
        version = "1.0"
        date = "2026-03-08"
        modified = "2026-03-08"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://WWW.DERP.CA/BLOG/GHOSTWEAVER-TAG124-POWERSHELL-RAT"
        author = "derp.ca"
        description = "GhostWeaver/Pantera persistence installer delivered via C2 iex command"
        category = "MALWARE"
        malware = "GHOSTWEAVER"
        mitre_att = "T1548.002"
        reference = "https://www.derp.ca/blog/ghostweaver-tag124-powershell-rat"
        triage_score = 10
        triage_description = "GhostWeaver/Pantera persistence installer with UAC bypass detected."
        yarahub_uuid = "90fcc0c0-91a8-46c0-8466-ecd287a0cfbd"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "78113996c94c5a1fcacba8da4ca86dde"

    strings:
        $uac_guid = "A6BFEA43-501F-456F-A845-983D3AD7B8F0" ascii wide nocase
        $uac_coget = "CoGetObject" ascii wide
        $uac_elevation = "Elevation:Administrator!new:" ascii wide

        $peb_func = "Masquerade-PEB" ascii wide
        $headless = "--headless powershell" ascii wide
        $headless_cmd = "--headless cmd" ascii wide

        $schtask_interval = "PT3M" ascii wide
        $schtask_desc = "Maintenance task" ascii wide
        $dpapi = "DataProtectionScope" ascii wide nocase
        $azure_prefix = /Azure[A-Za-z]+\.(ps1|log|jpg)/ ascii wide

    condition:
        ($uac_guid and ($uac_coget or $uac_elevation))
        or
        ($peb_func and ($headless or $headless_cmd))
        or
        ($schtask_interval and $schtask_desc and ($headless or $headless_cmd))
        or
        ($dpapi and ($headless or $headless_cmd) and $azure_prefix)
}
