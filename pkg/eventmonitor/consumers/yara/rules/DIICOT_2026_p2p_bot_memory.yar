/*
    YARA rule - DIICOT / Mexals 2026 generation, P2P bot module ("cache")
    Author: eFeSpain  |  2026-09-19

    SCANS PROCESS MEMORY, NOT FILES.

    This module is Go compiled and obfuscated with garble: its own vocabulary is
    ENCRYPTED IN THE FILE and only decrypted in memory. `strings` over the sample
    returns none of the giveaway text - not the Telegram domain, not the bot id,
    not the status panel. Verified: zero hits for every behavioural string, on
    the packed sample and on the UPX-unpacked one alike.

    Every anchor below was verified on a live node, detonated in an isolated
    network namespace with no route out, then frozen and dumped from
    /proc/PID/mem. Two runs: one idle, one with the local CLI exercised.

    Important: garble decrypts each literal ONLY when its code path runs. On an
    idle node the status-panel strings ($panel, $ver) are NOT in memory - they
    appear once the operator asks for status over Telegram. That is why they are
    not the primary anchor.

    False positives: none across 13 system processes of a clean Debian 13 VM.

    For file-based detection of the same kit, see the companion rule
    DIICOT_2026_kit_garble.
*/

rule DIICOT_2026_p2p_bot_memory
{
    meta:
        description    = "DIICOT/Mexals 2026 P2P bot - process memory (mesh, leader election, Telegram C2)"
        author         = "eFeSpain"
        author_url     = "https://efespain.com"
        date           = "2026-09-19"
        reference      = "https://blog.efespain.com/en/chapter-27/"
        family_page    = "https://blog.efespain.com/en/diicot/"
        malware_family = "DIICOT"
        scan_target    = "process memory (the sample is garble-obfuscated on disk)"
        method         = "anchors verified on a live node detonated in an isolated netns, memory dumped from /proc/PID/mem"
        caveat         = "garble decrypts each literal ONLY when the code path runs: the status-panel strings are absent from an idle node"
        tlp            = "clear"
        yarahub_uuid              = "bc58b493-0f05-4230-b00e-be969e8fe5de"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_link    = "https://blog.efespain.com/en/chapter-27/"
        yarahub_reference_md5     = "35ff872c6ec557c87f71bc2a483cf252"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"

    strings:
        // --- present from boot on a running node (verified) ---
        $salt  = "p2p-peers-salt-v1" ascii          // peer-store seed, unique to this family
        $pdat  = "peers.dat" ascii
        $tgdom = "api.telegram.org" ascii
        // --- runtime log lines (verified) ---
        $l1 = "cmds only from Telegram" ascii       // refuses local CLI once Telegram is up
        $l2 = "TG bot active" ascii
        $l3 = "take lead" ascii
        $l4 = "boot done" ascii
        $l5 = "tg:LEADER" ascii
        // --- operator command set (verified after the CLI is exercised) ---
        $c1 = "/peers" ascii
        $c2 = "/connect" ascii
        $c3 = "/leader" ascii
        $c4 = "/hub" ascii
        $c5 = "/check" ascii
        $c6 = "/update" ascii
        // --- only materialise once their code path runs; kept because they are decisive ---
        $panel = "DIICOT-BOTNET" ascii
        $ver   = "v2-update-1" ascii

    condition:
        $panel                                  // the bot naming itself: decisive
        or $salt                                // unique seed, present from boot
        or $l1                                  // unique phrase
        or 2 of ($l*)
        or ($ver and $tgdom)
        or ($tgdom and $pdat and 3 of ($c*))
}
