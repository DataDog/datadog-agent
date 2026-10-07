rule HKTL_Gsocket_Proc_Masquerade_Artifacts
{
    meta:
        description = "gsocket deployment artifact containing process-masquerade names"
        reference = "https://github.com/hackerschoice/gsocket/blob/beta/deploy/deploy.sh"
        author = "Peter Gabaldon"
        date = "2026-08-23"
        severity = "high"
        note = "Dual-use: the official upstream deploy.sh matches; correlate with execution and persistence evidence"
        yarahub_reference_link = "https://gist.github.com/PeterGabaldon/e5b9a6c4ad4e3278481fb5cb02d67a8c"
        yarahub_reference_md5 = "d7257d1dc7a1c36a8be60e166f771af0"
        yarahub_uuid = "d494d887-12b3-4d0b-bc51-3ebd13ddb457"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        // Verbatim from proc_name_arr[] in the beta deploy.sh.
        $n01 = "[kthreadd]" ascii wide
        $n02 = "[kstrp]" ascii wide
        $n03 = "[watchdogd]" ascii wide
        $n04 = "[ksmd]" ascii wide
        $n05 = "[kswapd0]" ascii wide
        $n06 = "[card0-crtc8]" ascii wide
        $n07 = "[mm_percpu_wq]" ascii wide
        $n08 = "[rcu_preempt]" ascii wide
        $n09 = "[kworker]" ascii wide
        $n10 = "[raid5wq]" ascii wide
        $n11 = "[slub_flushwq]" ascii wide
        $n12 = "[netns]" ascii wide
        $n13 = "[kaluad]" ascii wide
        $n14 = "sshd:" ascii wide

        // A process name alone is too generic; require a gsocket anchor.
        $g1 = "GS_PROC_HIDDENNAME"
        $g2 = { 8C CC FF D0 85 86 E0 EC }
        $g3 = "8xKd12TX"
        $g4 = "GSOCKET_PROC_HIDDENNAME"

    condition:
        1 of ($n*) and 1 of ($g*)
}
