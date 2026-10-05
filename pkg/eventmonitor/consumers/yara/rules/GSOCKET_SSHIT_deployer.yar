/*
 * GSOCKET_SSHIT -- abuse of THC Global Socket / ssh-it for persistent access
 * Drosera honeypot capture, 2026-08-02
 *
 * IMPORTANT: the tool being deployed here is legitimate. THC gsocket and
 * ssh-it are published security tools with lawful uses. This rule targets
 * the ATTACKER'S DEPLOYMENT WRAPPER -- specifically the exfil callback to
 * 192.253.248.9 -- not gsocket itself.
 *
 * Do NOT broaden this rule to match gsocket.io URLs or the gsocket binary.
 * Doing so will fire on legitimate administrator use.
 */

rule GSOCKET_SSHIT_deployer
{
    meta:
        description  = "Attacker wrapper deploying THC gsocket with exfil callback to 192.253.248.9"
        author       = "AfterPacket"
        date         = "2026-08-07"
        hash_sha256  = "22585585074dfaf862ab5bf0de958b99f146f0942b651af4f2f3ff3077b2513b"
        family       = "GSOCKET_SSHIT"
        tlp          = "TLP:WHITE"
        reference    = "https://github.com/Afterpacket/drosera-threat-intel"
        status       = "PRODUCTION"
        note         = "Targets the attacker wrapper only. THC gsocket is a legitimate tool."

        /* YARAhub / YARAify submission metadata.
         * reference_md5 is /da (sha256 22585585...), carrying the exfil
         * endpoint 192.253.248.9/gsocket/up.php. */
        yarahub_uuid              = "e7b02a58-4c1f-4d93-a6b7-8e05f2c47d31"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "5d3646759b4618540e0eca570f570d5b"
        yarahub_reference_link    = "https://github.com/Afterpacket/drosera-threat-intel"
        yarahub_author_twitter    = "@AfterPacket"
        yarahub_author_email      = "AfterPacketTru@protonmail.com"

    strings:
        /* High-confidence -- the operator's own infrastructure */
        $uniq1 = "192.253.248.9/gsocket/up.php" ascii
        $uniq2 = "http://192.253.248.9/gsocket/up.php" ascii

        /* C2 indicator */
        $c2_1 = "192.253.248.9" ascii

        /* Wrapper-specific variables -- not part of upstream gsocket */
        $cap1 = "GS_UPLOAD_URL" ascii
        $cap2 = "URL_DEPLOY" ascii
        $cap3 = "GS_OUTPUT_FILE" ascii

        /* Victim fingerprinting */
        $cap4 = "ifconfig.me" ascii

        /* Persistence artefacts. ~/.config/prng is the wrapper's own staging
         * directory -- it collides with nothing legitimate and is the best
         * single host indicator for this family. The systemd unit name is
         * likewise invented by the attacker, not by upstream gsocket. */
        $cap5 = ".config/prng" ascii
        $cap6 = "gsocket-watchdog.service" ascii
        $cap7 = "gsocket_watchdog.pid" ascii

    condition:
        filesize < 256KB and
        (
            /* HIGH: the exfil endpoint is unambiguous */
            any of ($uniq*) or

            /* MEDIUM: operator IP plus wrapper-specific variables.
             * Requires two wrapper markers so upstream gsocket alone
             * can never satisfy this branch. */
            ($c2_1 and 2 of ($cap*))
        )
}
