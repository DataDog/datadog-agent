rule win_amadey_a9f4 {

    meta:
        author                    = "Johannes Bader"
        date                      = "2022-11-17"
        description               = "matches unpacked Amadey samples"
        hash_md5                  = "25cfcfdb6d73d9cfd88a5247d4038727"
        hash_sha1                 = "912d1ef61750bc622ee069cdeed2adbfe208c54d"
        hash_sha256               = "03effd3f94517b08061db014de12f8bf01166a04e93adc2f240a6616bb3bd29a"
        malpedia_family           = "win.amadey"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "25cfcfdb6d73d9cfd88a5247d4038727"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "a9f41cd4-3f67-42fc-b310-e9b251c95fe4"

    strings:
        $pdb  = "\\Amadey\\Release\\Amadey.pdb"
        /*  Amadey uses multiple hex strings to decrypt the strings, C2 traffic
            and as identification. The preceeding string 'stoi ...' is added to
            improve performance.
        */
        $keys = /stoi argument out of range\x00\x00[a-f0-9]{32}\x00{1,16}[a-f0-9]{32}\x00{1,4}[a-f0-9]{6}\x00{1,4}[a-f0-9]{32}\x00/

    condition:
        uint16(0) == 0x5A4D and
        (
            $pdb or $keys
        )
}
