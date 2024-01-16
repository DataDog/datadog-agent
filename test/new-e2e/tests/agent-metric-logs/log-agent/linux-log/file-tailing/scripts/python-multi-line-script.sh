#!/bin/bash
cat > random-multi-line-logger.py << EOF
#!/usr/bin/env python3
import argparse
import logging
import random
from time import sleep

parser = argparse.ArgumentParser(description='Random Multiline logger.')
parser.add_argument('--log-file', dest='log_filename', default="/var/log/hello-world.log", help='Set logger to output to a logfile rather than stout')

args = parser.parse_args()

logging.basicConfig(filename=args.log_filename, format='%(asctime)s | %(levelname)s | %(message)s', level=logging.DEBUG)

log_count = 0
while True:
    # Single line log for first 60 logs and then 50% chance for either single or multi-line
    if log_count <= 60 or random.randint(0,1):
        logging.debug('This is a debug log that shows a log that can be ignored.')
    else:
        logging.error('An error is\nusually an exception that\nhas been caught and not handled.')
    sleep(1)
    log_count += 1
EOF
sudo mv random-multi-line-logger.py /usr/bin/random-multi-line-logger.py
sudo chmod +x /usr/bin/random-multi-line-logger.py
