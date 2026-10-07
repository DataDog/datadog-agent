
private rule Seedhook_MachO
{
    condition:
        uint32(0) == 0xfeedfacf or uint32(0) == 0xcffaedfe or   // thin 64-bit
        (uint32(0) == 0xbebafeca and uint32be(4) < 8)          // fat (CAFEBABE, few slices; excludes Java .class)
}

rule Seedhook_LoaderAgent_wsecurite
{
    meta:
        yarahub_uuid = "7174e96b-bebf-4008-a921-3e8a90da0165"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "ab8b6bcd2a7fdce51983a0c46061252c"
        description = "Seedhook/MacSync loader_agent 'safeguard' build with XOR-0xAA encoded C2 <random label>.wsecurite.com (reverse tunnel on :443)"
        actor = "Seedhook (MacSync Stealer cluster)"
        reference_sha256 = "6e4b84389afb4683e19f921ce3f54703fb6bb146fbdbffac3e398894074c0fa0"
        c2 = "ahJNo6F501Z6JD1NNAH9bc236ifFkePeDKzyvysHi.wsecurite.com:443"
        date = "2026-09-18"
        tlp = "CLEAR"

    strings:
        // match on the registered domain so a rotated subdomain label still hits
        $c2 = ".wsecurite.com" xor(0xAA)
        $g  = "/loader/gate" xor(0xAA)

    condition:
        Seedhook_MachO and all of them
}
