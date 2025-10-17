import schema_pb2
from google.protobuf import json_format


IN_FILE = 'pkg/config/schema/fields.message'
OUT_FILE = 'pkg/config/gen/fields.bin'


def main():
    fp = open(IN_FILE, 'r')
    content = fp.read()
    fp.close()

    new_schema = schema_pb2.Schema()
    message = json_format.Parse(content, new_schema)

    bin = message.SerializeToString()
    fout = open(OUT_FILE, 'wb')
    fout.write(bin)
    fout.close()


if __name__ == '__main__':
    main()