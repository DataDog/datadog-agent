rule WIN_JS_Downloader_ActiveX_XorChain_FakeCLI
{
    meta:
        description = "JScript downloader - URL dechiffree par chaine XOR, second stage via Microsoft.XMLHTTP + eval, faux CLI en leurre"
        author = "Marjoriefort"
        yarahub_reference_md5 = "9256e081b742c1bafa79f73e334092d2"
        date = "2026-09-19"

        yarahub_uuid = "a763e2b1-2afa-45be-9779-ae80850e443b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $ax  = "Microsoft.XMLHTTP"
        $ev  = "eval(xhr.responseText)"
        $xo1 = "v=(v+s)^k2"
        $xo2 = "String.fromCharCode(v^k1)"
        $cli = "fmtMoney"

    condition:
        $ax and $ev and 1 of ($xo1, $xo2, $cli) and filesize < 500KB
}
