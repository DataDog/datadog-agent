rule ECHidna_Delivery_LNK
{
    meta:
        id = "uapjdWLMJKm7q5p5G1fb2w"
        fingerprint = "e6804e6d987e6ee6434360637ecc4598252fb245cc8aa84cb17d86de992da52f"
        version = "1.0"
        date = "2026-03-09"
        modified = "2026-03-09"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://GITHUB.COM/KIRKDERP/YARA"
        author = "kirkderp"
        description = "ECHidna LNK delivery stager that sideloads the BoringSSL wrapper via rundll32."
        category = "MALWARE"
        malware = "ECHIDNA"
        mitre_att = "T1204.002"
        reference = "https://github.com/kirkderp/yara"
        triage_score = 10
        triage_description = "ECHidna LNK delivery stager detected."
        yarahub_uuid = "f5bf9155-1428-4649-85a6-4d7a872a93db"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "5321d27958a11f9744cf23be0204f386"

    strings:
        // DLL filename pattern used for staging
        $ms_service = "MS-Service" wide

        // SSL_version export name passed to rundll32
        $ssl_version = "SSL_version" wide

        // desktop.ini used as disguised DLL source
        $desktop_ini = "desktop.ini" wide

        // ExecutionPolicy bypass string evasion (splits "bypass")
        $bypass_evasion = "by+pass" wide

        // Secondary C2 tracking beacon
        $tracking = "jlmin.cc" wide

    condition:
        uint32(0) == 0x0000004c  // LNK magic
        and filesize < 10KB
        and (
            ($ms_service and $ssl_version)
            or ($desktop_ini and $ssl_version)
            or ($bypass_evasion and ($ms_service or $ssl_version))
            or ($tracking and $desktop_ini)
        )
}
