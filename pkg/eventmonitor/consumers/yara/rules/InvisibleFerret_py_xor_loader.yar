/*
 * InvisibleFerret (ContagiousInterview) stage-1 loader -- structural rule (XOR variant).
 *
 * The auto string-rule generator ranks byte-identical strings by share.
 * InvisibleFerret re-obfuscates each sample (variable names permuted), so no
 * single decode string clears the share gate -- yet the structure of the
 * stage-1 loader is invariant. This rule matches that structure with variable
 * names wildcarded, so one rule covers every rename variant.
 *
 * Coverage on the 22-sample shared corpus: 11/22 (real-file coverage likely
 * higher). Static analysis only; samples never executed. FP rate NOT yet
 * evaluated -- upload to YARAify, watch matches, tighten.
 *
 * NOTE: the zlib-lambda variant is a separate one-rule file
 * (script.zlib.yaraify.yar) because YARAhub accepts one rule per file.
 */

rule InvisibleFerret_py_xor_loader
{
    meta:
        description = "InvisibleFerret stage-1 loader: 8-byte rolling-key XOR deobfuscator + exec (var names wildcarded)"
        family = "InvisibleFerret"
        actor = "ContagiousInterview"
        filetype = "script"
        method = "structural regex (rename-invariant)"
        author = "hunts-yara-struct"
        date = "2026-09-03"
        note = "operands/var names wildcarded; static; FP rate not evaluated; 11/22 shared corpus"
        yarahub_uuid = "9e5b38bb-67e4-45af-86f2-8b2ebaded723"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "b8b1477e40c32186bccaa4cb10254977"
        hash0 = "08979f0c983afe55529877ded40f7f6c3276a63ed16e112f0051b5dc2a3c8d02"
        hash1 = "2633d3f611fb783d0a6ecbdf903853c18b17fbfbe65997acd1fd24954e3f8b84"
        hash2 = "2d96b0a4e5e6511cbdde4bcc769f6bf46e4598fed106cfb0f9d24c450d641f60"
        hash3 = "2f403bc9f148f4bf83c2e5ad2b217225997f9b9e3946ed64d4ea1792321cd2cd"
        hash4 = "2fc2ff396c90c8da4042b4ab72b5be4dbf2a47ad105bfa287f285cecdc6ba2c7"
        hash5 = "3adf5ddaacfcc2d796b40fb386f57c5e6a8dff01083fa921abc0c799cc30551e"
    strings:
        // k=i&7; c=chr(d[i]^ord(sk[k]))   (buffer/index/key/out vars all renamed per sample)
        $xor  = /&7;\w{1,3}=chr\(\w{1,4}\[\w{1,4}\]\^ord\(\w{1,4}\[\w{1,4}\]\)\)/
        $b64  = "b64decode("
        $exec = "exec("
    condition:
        filesize < 2MB and $xor and $b64 and $exec
}
