/*
 * PERLBOT_SHELLBOT -- Perl IRC shellbot
 * Drosera honeypot capture, 2026-08-02 .. 2026-08-07
 *
 * Covers all three captured samples (/duba, /dodu, /gots), including
 * /dodu -- which carries NO VirusTotal record despite being the same size
 * as /duba, which 29 of 60 engines flag. The undetected twin is the
 * higher-value hunt target.
 *
 * NOTE: the bot accepts a C2 override as $ARGV[0], so the hardcoded server
 * is the default rather than a guarantee. The structural strings below
 * survive a redirected C2; the IP string does not.
 */

rule PERLBOT_SHELLBOT_irc
{
    meta:
        description  = "Perl IRC shellbot, C2 213.139.77.150:6667, randomised nick and process name"
        author       = "AfterPacket"
        date         = "2026-08-07"
        hash_sha256  = "03a4f492af99d2048f713081560b8fa45312e594e8439eefa714f0c67a1e0550"
        hash_sha256_2 = "fe6b7b4c09ec79caa51ee7a7c46bbfdb11b0c7e0a380292675fb5ac6ce2b26c2"
        hash_sha256_3 = "fd93b4f7bd5e8360ecd6e782dd445356728e698b9f25e335131b28bcdd3e56e3"
        family       = "PERLBOT_SHELLBOT"
        tlp          = "TLP:WHITE"
        reference    = "https://github.com/Afterpacket/drosera-threat-intel"
        status       = "PRODUCTION"

        /* YARAhub / YARAify submission metadata.
         * reference_md5 is /duba (sha256 03a4f492...), the VT 29/60 sample
         * whose config block satisfies $c2_2 outright. */
        yarahub_uuid              = "18c3d6f2-9e7b-4a45-90d8-3b62e5f7a1c9"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "4c78efb135174df31abadaa95c43d470"
        yarahub_reference_link    = "https://github.com/Afterpacket/drosera-threat-intel"
        yarahub_author_twitter    = "@AfterPacket"
        yarahub_author_email      = "AfterPacketTru@protonmail.com"

    strings:
        /* C2 indicators -- defeated by an $ARGV[0] override, so never sole-condition */
        $c2_1 = "213.139.77.150" ascii
        $c2_2 = "my $server = '213.139.77.150'" ascii
        $c2_3 = "my $port = '6667'" ascii

        /* Structural markers -- survive C2 redirection */
        $s1 = "rircname" ascii
        $s2 = "$rps[rand scalar" ascii
        $s3 = "[rand scalar @rircname]" ascii
        $s4 = "$server=\"$ARGV[0]\" if $ARGV[0]" ascii

        /* Operator constants. Identical in all three samples across BOTH C2
         * servers (213.139.77.150 and 213.177.179.11) -- which is what proves
         * one operator runs both. Unlike the nick, these are not randomised,
         * and unlike the server they survive the $ARGV[0] override. They are
         * the most durable indicators this family has. */
        $op1 = "@admins = (\"MAD\")" ascii
        $op2 = "@channels = (\"#mot\")" ascii

        /* Perl script anchor */
        $perl = "#!/usr/bin/perl" ascii

    condition:
        filesize < 512KB and
        (
            /* HIGH: the full config-block assignment */
            $c2_2 or

            /* HIGH: both operator constants together. Catches every sample in
             * this campaign regardless of which C2 it points at, including
             * after an $ARGV[0] override that defeats every IP-based branch. */
            all of ($op*) or

            /* HIGH: two structural markers -- C2-redirection resistant */
            2 of ($s*) or

            /* MEDIUM: Perl script carrying the C2 and one structural marker */
            ($perl and any of ($c2_*) and any of ($s*)) or

            /* MEDIUM: Perl script joining the operator's channel */
            ($perl and any of ($op*))
        )
}
