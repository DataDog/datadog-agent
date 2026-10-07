rule ASP_VBScript_Encoded_Webshell {
    meta:
        description = "Encoded VBScript ASP page (Microsoft Script Encoder) - webshell-grade legacy"
        author = "Marjoriefort"
        date = "2026-09-13"
        reference = "InTheWild.0438 / tennc 016_shell.asp + 034_CyberSpy5"
        confidence = "medium"
        yarahub_uuid            = "5e4db9e8-d38d-4af4-8b71-63c6513869a1"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "fe0c5bbe9cb3333f671b7ef02d883b4a"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $h   = "<%@ LANGUAGE = VBScript.Encode %>"
        $enc = "@#@~^"
    condition:
        $h and $enc
}

