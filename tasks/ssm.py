import base64
import json
import os
import tempfile


ssm_command = "aws.exe ssm get-parameter --name {} --with-decryption --region us-east-1"
ssm_param_password = "keygen.dd_win_agent_codesign.password"
ssm_param_pfx_part1 = "keygen.dd_win_agent_codesign.pfx_b64_0"
ssm_param_pfx_part2 = "keygen.dd_win_agent_codesign.pfx_b64_1"

def get_value_of_param(ctx, param):
    full_command = ssm_command.format(param)
    result = ctx.run(full_command, hide='stdout')
    # if there's an exception, just let it pass through

    if not result.ok:
        print("result not ok")
        return None
     
    json_out = result.stdout
    j = json.loads(json_out)
    
    invalidparms = j.get("InvalidParameters")
    paramkey = j.get("Parameter")
    if invalidparms:
        print("Param is invalid {}".format(invalidparms))
        return None

    val = paramkey.get("Value")
    print("Length of paramkey {}".format(len(val)))
    return val

def get_signing_cert(ctx):
    pfx_b64_encoded_part1 = get_value_of_param(ctx, ssm_param_pfx_part1)
    if not pfx_b64_encoded_part1:
        return None
    pfx_b64_encoded_part2 = get_value_of_param(ctx, ssm_param_pfx_part2)
    if not pfx_b64_encoded_part2:
        return None
    pfx_b64_encoded = pfx_b64_encoded_part1 + pfx_b64_encoded_part2
    enclen = len(pfx_b64_encoded)
    print("encoded length {}".format(enclen))
    pfx_b64_decoded = base64.b64decode(pfx_b64_encoded)

    f, fn = tempfile.mkstemp() # default mode is binary, which we want
    os.write(f, pfx_b64_decoded)
    os.close(f)
    return fn

def get_pfx_pass(ctx):
    return get_value_of_param(ctx, ssm_param_password)

    


