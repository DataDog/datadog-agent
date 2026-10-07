/*
    YARA rule - DIICOT / Mexals 2026 generation, whole kit (bot + loader)
    Author: eFeSpain  |  2026-09-19

    Scans FILES. The kit is Go compiled and obfuscated with garble, so none of
    its behavioural strings survive in the binary. What does survive is the set
    of obfuscated package names garble generated for this build lineage - the
    obfuscation itself becomes the signature.

    Verified: these identifiers are present in two distinct samples of the kit
    (the P2P bot and the loader) and absent from legitimate Go binaries - the
    control was docker + containerd, 64093 identifiers, no overlap.

    Caveat, and it matters: garble derives these names from a build seed. The day
    the operator re-seeds, this rule stops matching. It identifies a build
    lineage, not the family's behaviour.

    Does NOT fire on the UPX-packed sample: unpack first (YARAify does this with
    `unpack`), or use the companion rule DIICOT_2026_p2p_bot_memory on memory.
*/

rule DIICOT_2026_kit_garble
{
    meta:
        description    = "DIICOT/Mexals 2026 kit - garble package names shared by the bot and the loader"
        author         = "eFeSpain"
        author_url     = "https://efespain.com"
        date           = "2026-09-19"
        reference      = "https://blog.efespain.com/en/chapter-27/"
        family_page    = "https://blog.efespain.com/en/diicot/"
        malware_family = "DIICOT"
        scan_target    = "unpacked ELF file (does not fire on the UPX-packed sample)"
        caveat         = "garble derives these names from a build seed: a re-seeded build will not match"
        validation     = "0 hits on 1393 system binaries and on 5 unrelated malware families; 10 hits, all DIICOT 2026 modules"
        tlp            = "clear"
        yarahub_uuid              = "65dc87ef-4d6c-4d2a-b25e-3fc52bbc263f"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_link    = "https://blog.efespain.com/en/chapter-27/"
        yarahub_reference_md5     = "35ff872c6ec557c87f71bc2a483cf252"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"

    strings:
        $g1 = "AXImNAJor24q" ascii
        $g2 = "B9yNPiSKMPYX" ascii
        $g3 = "BSgmr3gqwXQ0" ascii
        $g4 = "CPYNrVWM6oMe" ascii
        $g5 = "CQsqhmrWCaX0" ascii
        $g6 = "CYkplmB9PYZW" ascii
        $g7 = "AsTBAr8oaZK"  ascii
        $g8 = "Bauw6mS3rtB"  ascii

    condition:
        uint32(0) == 0x464c457f          // ELF
        and filesize > 1MB
        and 4 of ($g*)
}
