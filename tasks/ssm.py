import atexit
import base64
import json
import os
import tempfile

ssm_command = "aws.cmd ssm get-parameter --name {} --with-decryption --region us-east-1"
ssm_param_password = "keygen.dd_win_agent_codesign.password"
ssm_param_pfx_part1 = "keygen.dd_win_agent_codesign.pfx_b64_0"
ssm_param_pfx_part2 = "keygen.dd_win_agent_codesign.pfx_b64_1"


def get_value_of_param(ctx, param):
    full_command = ssm_command.format(param)
    # With default values, we can get "Connect timeout on endpoint" errors.
    # AWS suggests increasing timeout values when fetching credentials on
    # an EC2 instance configured with an IAM role.
    # See: https://boto3.amazonaws.com/v1/documentation/api/latest/guide/configuration.html
    env = {
        "AWS_METADATA_SERVICE_TIMEOUT": "5",  # 5 seconds instead of 1 by default
        "AWS_METADATA_SERVICE_NUM_ATTEMPTS": "5",  # 5 attempts instead of 1 by default
    }
    result = ctx.run(full_command, env=env, hide='stdout')
    # if there's an exception, just let it pass through

    if not result.ok:
        print("result not ok")
        return None

    json_out = result.stdout
    j = json.loads(json_out)

    invalidparms = j.get("InvalidParameters")
    paramkey = j.get("Parameter")
    if invalidparms:
        print(f"Param is invalid {invalidparms}")
        return None

    val = paramkey.get("Value")
    print(f"Length of paramkey {len(val)}")
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
    print(f"encoded length {enclen}")
    pfx_b64_decoded = base64.b64decode(pfx_b64_encoded)

    f, fn = tempfile.mkstemp()  # default mode is binary, which we want
    os.write(f, pfx_b64_decoded)
    os.close(f)

    def delete_pfxfile():
        os.remove(fn)

    atexit.register(delete_pfxfile)

    return fn


def get_pfx_pass(ctx):
    return get_value_of_param(ctx, ssm_param_password)
