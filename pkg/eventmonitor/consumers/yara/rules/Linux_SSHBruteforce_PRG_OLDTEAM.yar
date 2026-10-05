rule Linux_SSHBruteforce_PRG_OLDTEAM
{
    meta:
        description = "Linux SSH brute-force toolkit (PRG / OLDTEAM), often masquerading as image"
        author = "noopoo/0XFF1"
        date = "2026-02-06"
        malware_family = "PRG-OLDTEAM"
        yarahub_license = "CC0 1.0"
        yarahub_uuid = "d3c74bf4-4cf9-4ff2-b6c6-c0767888e68c"
        reference = "MalwareBazaar upload"
        yarahub_reference_md5 = "d1ca004fbda5fedcd6583b09b679c581"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        /* Identity / Ego */
        $id1 = "PRG-oldTeam" ascii nocase
        $id2 = "OLDTEAM" ascii
        $id3 = "LET'S MAKE SOME ADMINS TO CRY" ascii nocase
        $id4 = "CREATED BY PRG" ascii


        /* Config / workflow */
        $cfg1 = "ips.lst" ascii
        $cfg2 = "pass.lst" ascii
        $cfg3 = "uidThreads" ascii
        $cfg4 = "usrThreads" ascii
        $cfg5 = "Banner grabber starting" ascii

        /* SSH / libssh2 internals */
        $ssh1 = "Invalid MAC received" ascii
        $ssh2 = "Channel open failure" ascii
        $ssh3 = "libssh2" ascii
        $ssh4 = "Unable to send channel data" ascii

        /* Packaging / structure */
        $pkg1 = ".stx/" ascii
        $pkg2 = "ustar" ascii
        $pkg3 = "gzip compressed data" ascii

    condition:
        /* Identity is mandatory */
        1 of ($id*) and

        /* Must show brute-force behavior */
        2 of ($cfg*) and
        2 of ($ssh*) and

        /* Plus archive / delivery context */
        1 of ($pkg*)
}
