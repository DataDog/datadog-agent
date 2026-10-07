rule JS_Obfuscation_Base256_Decoder {
    meta:
        description = "Detects JavaScript phishlets using base-256 integer array decoding to hide payload"
        author = "Serhii Kocherhan"
        date = "2026-09-10"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "88bdd1e5-9e61-4879-a99b-eb5a06747417"
        yarahub_license = "CC0 1.0"
        yarahub_reference_md5 = "73e66bdef0fd31d8306e334112bf383c"
        reference = "Analysis of array integer-to-string unpacking routine"
        severity = "Medium"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        // Characteristic mathematical unpacking elements
        $math_pow3 = "Math.pow(256,3)" ascii nocase
        $math_pow2 = "Math.pow(256,2)" ascii nocase
        
        // Core decoding and output functions
        $char_code = "String.fromCharCode" ascii
        $doc_write = "document.write" ascii
        
        // Typical array initialization pattern
        $array_init = "new Array" ascii

    condition:
        // Matches scripts combining the base-256 math decoding logic with character conversion
        filesize < 2MB and
        (
            ($math_pow3 and $math_pow2 and $char_code) or 
            (any of ($math_pow*) and $char_code and $doc_write) or
            ($array_init and $char_code and any of ($math_pow*))
        )
}