rule SUSP_VBS_Randomized_ActiveX_Downloader {
    meta:
        description = "Dropper VBS obfusque : ActiveXObject a identifiant randomise (20 MAJ), SaveToFile, .Run"
        author      = "Marjoriefort"
        date        = "2026-09-16"
        reference   = "Veille fraicheur 2026-09-14 / grappe VBS randomisee"
        confidence  = "medium-high"
        yarahub_uuid            = "cf3cabf6-6539-4cfd-9da0-1fd8e92227bd"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "c917bff2167fcd8d97ad6eec8d96d0fc"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = /ActiveXObject\([A-Z]{15,25}\)/ ascii wide
        $b = "SaveToFile" ascii wide nocase
        $c = ".Run" ascii wide
    condition:
        $a and 1 of ($b, $c)
}
