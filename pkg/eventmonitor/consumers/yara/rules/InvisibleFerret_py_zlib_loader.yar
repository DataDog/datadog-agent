/*
 * InvisibleFerret (ContagiousInterview) stage-1 loader -- structural rule (zlib variant).
 *
 * Rename-invariant match for the zlib.decompress(b64decode(reversed)) lambda
 * loader variant. Coverage on the 22-sample shared corpus: 2/22. Companion to
 * script.yaraify.yar (XOR variant) -- split into its own file because YARAhub
 * accepts one rule per file. Static analysis only; samples never executed.
 */

rule InvisibleFerret_py_zlib_loader
{
    meta:
        description = "InvisibleFerret stage-1 loader: zlib.decompress(b64decode(reversed)) lambda variant"
        family = "InvisibleFerret"
        actor = "ContagiousInterview"
        filetype = "script"
        method = "structural regex (rename-invariant)"
        author = "hunts-yara-struct"
        date = "2026-09-03"
        note = "static; FP rate not evaluated; 2/22 shared corpus -- upload alongside xor_loader"
        yarahub_uuid = "f50c68cf-1770-4d3b-91fc-b390a2a4d010"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "71fb5cb5f677a5c80f6665c20bab3f97"
        hash0 = "08979f0c983afe55529877ded40f7f6c3276a63ed16e112f0051b5dc2a3c8d02"
        hash1 = "2633d3f611fb783d0a6ecbdf903853c18b17fbfbe65997acd1fd24954e3f8b84"
    strings:
        // _ = lambda __ : __import__('zlib').decompress(__import__('base64').b64decode(__[::-1]))
        $z1 = /__import__\(['"]zlib['"]\)\.decompress/
        $z2 = /b64decode\(\w{1,4}\[::-1\]\)/
    condition:
        filesize < 2MB and all of them
}
