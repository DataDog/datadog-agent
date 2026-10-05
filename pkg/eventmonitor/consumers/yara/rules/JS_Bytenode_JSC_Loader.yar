rule JS_Bytenode_JSC_Loader {
    meta:
        description = "bytenode loader executing compiled V8 bytecode (app.protected.jsc) - payload invisible to string scanning"
        author = "Marjoriefort"
        date = "2026-09-13"
        reference = "InTheWild.0438 / tag_js 0fc4a52a"
        confidence = "medium"
        yarahub_uuid            = "e60a6144-e24c-4cd9-aa8c-ba13b0d7c701"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "188f2d1f47419006b9c40d4819709abd"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $b = "require('bytenode')"
        $j = "app.protected.jsc"
    condition:
        $b and $j
}

