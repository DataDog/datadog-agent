/*
    CVE-2026-2441: Chrome CSSFontFeatureValuesMap Use-After-Free
    Detects HTML pages exploiting iterator invalidation in @font-feature-values

    Formatted for YARAify (abuse.ch) deployment with required yarahub_* metadata.
    Deploy via: https://yaraify.abuse.ch/api/ with Auth-Key header.

    Author: sec_toolkit
    Reference: CVE-2026-2441, Chromium commit 63f3cb4864c64c677cd60c76c8cb49d37d08319c
*/

rule CVE_2026_2441_FontFeatureValues_UAF
{
    meta:
        author = "sec_toolkit"
        description = "Detects HTML/JS exploiting CVE-2026-2441: CSSFontFeatureValuesMap iterator invalidation UAF in Chrome < 145.0.7632.75. The exploit combines CSS @font-feature-values rules with JavaScript that iterates and mutates the styleset map simultaneously, causing HashMap reallocation and heap corruption leading to RCE inside sandbox."
        severity = "critical"
        cve = "CVE-2026-2441"
        cwe = "CWE-416"
        reference = "https://chromereleases.googleblog.com/2026/02/stable-channel-update-for-desktop_13.html"

        // YARAify / YARAhub required metadata
        date = "2026-02-17"
        yarahub_uuid = "5443f372-24b8-4281-be98-d2a9a480ff19"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "ac1947bd6d33ac9291a9d1a18a96dba9"

    strings:
        // === CSS TRIGGER: @font-feature-values at-rule ===
        $css_ffv = "@font-feature-values" ascii nocase

        // === CSS FEATURE BLOCKS (at least one needed to populate the map) ===
        $css_block1 = "@styleset" ascii nocase
        $css_block2 = "@stylistic" ascii nocase
        $css_block3 = "@swash" ascii nocase
        $css_block4 = "@ornaments" ascii nocase
        $css_block5 = "@annotation" ascii nocase
        $css_block6 = "@character-variant" ascii nocase

        // === JS MAP ACCESS: getting the CSSFontFeatureValuesMap object ===
        $js_map1 = ".styleset" ascii
        $js_map2 = ".stylistic" ascii
        $js_map3 = ".swash" ascii
        $js_map4 = ".ornaments" ascii
        $js_map5 = ".annotation" ascii
        $js_map6 = ".characterVariant" ascii

        // === JS ITERATOR CREATION ===
        $js_iter1 = ".entries()" ascii
        $js_iter2 = ".keys()" ascii
        $js_iter3 = ".values()" ascii
        $js_iter4 = ".forEach(" ascii
        $js_iter5 = "for (const" ascii
        $js_iter6 = "for (let" ascii
        $js_iter7 = "for (var" ascii
        $js_iter8 = ".next()" ascii

        // === JS MAP MUTATION (the actual trigger) ===
        $js_mutate1 = ".delete(" ascii
        $js_mutate2 = ".set(" ascii
        $js_mutate3 = ".clear()" ascii

        // === BONUS: cssRules access pattern (alternative trigger path) ===
        $js_rules1 = "cssRules" ascii
        $js_rules2 = "styleSheets" ascii
        $js_rules3 = "font-feature-values" ascii

    condition:
        filesize < 5MB and
        (
            // PRIMARY: CSS @font-feature-values + JS map access + iterator + mutation
            (
                $css_ffv and
                1 of ($css_block*) and
                1 of ($js_map*) and
                1 of ($js_iter*) and
                1 of ($js_mutate*)
            )
            or
            // SECONDARY: CSS trigger + cssRules access + map access + mutation
            (
                $css_ffv and
                1 of ($js_rules*) and
                1 of ($js_map*) and
                1 of ($js_mutate*)
            )
            or
            // TERTIARY: cssRules + font-feature-values string + iterator + mutation
            (
                $js_rules1 and
                $js_rules3 and
                1 of ($js_map*) and
                1 of ($js_iter*) and
                1 of ($js_mutate*)
            )
        )
}
