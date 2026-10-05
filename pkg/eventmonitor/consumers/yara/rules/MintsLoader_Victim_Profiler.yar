rule MintsLoader_Victim_Profiler
{
    meta:
        id = "1NXLHyr8lbJ1fnH1QXX29d"
        fingerprint = "d6ef5c51d0a6dda0952fab0cae1ca7f242a12dc63946f6190a974759e80b5613"
        version = "1.0"
        date = "2026-03-08"
        modified = "2026-03-08"
        status = "RELEASED"
        sharing = "TLP:CLEAR"
        source = "HTTPS://WWW.DERP.CA/BLOG/GHOSTWEAVER-TAG124-POWERSHELL-RAT"
        author = "derp.ca"
        description = "MintsLoader/TAG-124 victim profiler - sandbox detection and scoring"
        category = "MALWARE"
        malware = "MINTSLOADER"
        mitre_att = "T1497.001"
        reference = "https://www.derp.ca/blog/ghostweaver-tag124-powershell-rat"
        triage_score = 10
        triage_description = "MintsLoader/TAG-124 sandbox scoring and victim profiling detected."
        yarahub_uuid = "a0d9ef63-d290-40b1-b87f-6b28e6a4c9fa"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "78113996c94c5a1fcacba8da4ca86dde"

    strings:
        $check_vm = "IsVirtualMachine" ascii wide nocase
        $check_gpu = "AdapterDACType" ascii wide nocase
        $check_cache = "CacheMemory" ascii wide nocase
        $check_video = "Win32_VideoController" ascii wide nocase

        $amsi = "AmsiOpenSession" ascii wide

        $callback_htr = /[a-z0-9]{5,15}htr[a-z0-9]{3,8}\.php/ ascii wide
        $callback_param_key = "&key=" ascii wide
        $callback_param_s = "&s=" ascii wide

        $dga4_charset = "'abcdefghijklmn'" ascii wide

    condition:
        (($check_vm or $amsi) and ($check_gpu or $check_cache) and $check_video)
        or
        ($callback_htr and $callback_param_key and $callback_param_s)
        or
        ($dga4_charset and ($check_vm or $check_gpu or $check_cache))
}
