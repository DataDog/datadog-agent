/*
 * SHARPEXT (Kimsuky) browser-extension JS beacon -- structural rule.
 *
 * SHARPEXT is a malicious Chromium extension that POSTs stolen data to a C2
 * and runs the response via eval(). Blocked from the auto string-rule path
 * only by the 8-sample minimum (4 samples collected). The beacon is anchored
 * on Web API identifiers (XMLHttpRequest / this.responseText / readyState),
 * which survive variable renaming -- so the rule is rename-invariant.
 *
 * Static analysis only; samples never executed. FP note: eval(this.responseText)
 * over XHR appears in some legacy AJAX code, so the rule requires the POST-and-
 * eval beacon shape, not eval alone. Upload to YARAify, watch matches, tighten.
 */

rule SHARPEXT_js_xhr_eval_beacon
{
    meta:
        description = "SHARPEXT malicious browser extension: XHR POST C2 beacon that eval()s the response"
        family = "SHARPEXT"
        actor = "Kimsuky"
        filetype = "script"
        method = "structural (Web API anchors, rename-invariant)"
        author = "hunts-yara-struct"
        date = "2026-09-03"
        note = "static; FP rate not evaluated; 4 shared-corpus samples; upload then tighten"
        yarahub_uuid = "3fb4a53a-3cb5-40a6-845a-8b87e6c358d2"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "06aa5716826b27100a31f9a875f911d0"
        hash0 = "014206412e19a506fe054d62850f52b657e3b287f7368a556483f99877ede873"
        hash1 = "11b99f460bf14c902083d2c9559da6f65ab376bcde5c63919a569ad5b5812d3d"
        hash2 = "971a630914ba9da76674a3157bff05a7a5bd0e09814b366da982bf6cd3204591"
        hash3 = "a4daa30a2ef6943d8eec7759246f6584bfd679b094cb8b66302355500a036b9a"
    strings:
        $xhr    = "new XMLHttpRequest()"
        $eval   = "eval(this.responseText)"
        $ready  = /\.readyState\s*==\s*4\s*&&\s*[\w.]{1,20}\.status\s*==\s*200/
        $form   = "application/x-www-form-urlencoded"
        // .open("POST", <var>)  -- var name wildcarded
        $post   = /\.open\(["']POST["'],\s*\w{1,20}\)/
    condition:
        filesize < 500KB and $eval and 2 of ($xhr, $ready, $form, $post)
}
