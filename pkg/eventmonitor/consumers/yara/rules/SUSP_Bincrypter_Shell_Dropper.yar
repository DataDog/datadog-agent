rule SUSP_Bincrypter_Shell_Dropper
{
    meta:
        description = "bincrypter-obfuscated shell dropper (THC bincrypter)"
        author = "Peter"
        date = "2026-09-18"
        reference = "https://github.com/hackerschoice/bincrypter"
        yarahub_uuid = "23fd62b7-7a7a-47f6-a3c0-7a0440cb214b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "53f5348430a4c30387fa019bc403e2c3"
        reference_filename = "netd.sh"

    strings:
        // _bc_obbell(): empty command substitutions spliced inside keywords
        // so eval/echo/perl/openssl never appear as literal tokens.
        $ob_bs  = { 60 21 20 3A 26 26 08 23 60 }
        $ob_bel = { 60 3A 7C 7C 07 60 }

        // fragments of the loader that survive the splicing
        $lang  = "LANG=C pe"
        $print = ":pr"

    condition:
        uint16(0) == 0x2123
        and filesize > 3072
        and ( #ob_bs + #ob_bel ) > 10
        and $lang
        and $print
}
