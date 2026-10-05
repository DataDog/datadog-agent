rule MintsLoader_JS_Dropper
{
    meta:
        id = "6SZfp0qlIsOShAB3dnLBkq"
        fingerprint = "943312f05cbf3672d37e8702198077afea03740657433601284fb6096a762514"
        version = "1.0"
        date = "2026-03-08"
        modified = "2026-03-08"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://WWW.DERP.CA/BLOG/GHOSTWEAVER-TAG124-POWERSHELL-RAT"
        author = "derp.ca"
        description = "MintsLoader/TAG-124 JavaScript dropper - XOR-obfuscated, delivers PowerShell"
        category = "MALWARE"
        malware = "MINTSLOADER"
        mitre_att = "T1059.007"
        reference = "https://www.derp.ca/blog/ghostweaver-tag124-powershell-rat"
        triage_score = 10
        triage_description = "MintsLoader/TAG-124 XOR-obfuscated JavaScript dropper detected."
        yarahub_uuid = "c1321fda-76c4-4c62-802e-45fc8b8a8ad1"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "78113996c94c5a1fcacba8da4ca86dde"

    strings:
        $cc_on = "//@cc_on" ascii

        $obf_func = /a0_0x[0-9a-fA-F]{3,8}/ ascii
        $str_mlhttp = "MLHTT" ascii
        $str_activex = "eXObj" ascii
        $str_wsh = "'WSH'" ascii
        $str_post = "'POST'" ascii
        $str_eval = "'eval'" ascii

        $delivery_url = /\/1\.php\?s=[0-9a-f\-]{8,40}/ ascii
        $delivery_url2 = /\/1\.php\?s=flibabc[0-9]{1,3}/ ascii

        $activex_shell = "WScript.Shell" ascii wide

    condition:
        filesize < 50KB
        and $cc_on
        and (
            (#obf_func > 10 and 3 of ($str_*))
            or
            (($delivery_url or $delivery_url2) and $activex_shell)
        )
}
