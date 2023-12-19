#!/bin/bash
cat > random-logger.py << EOF
#!/usr/bin/env python3
import argparse
import logging
import random
from time import sleep

parser = argparse.ArgumentParser(description='Random Multiline logger.')
parser.add_argument('--log-file', dest='log_filename', default="/var/log/hello-world.log", help='Set logger to output to a logfile rather than stout')

args = parser.parse_args()

logging.basicConfig(filename=args.log_filename, format='my-log-entry %(asctime)s | %(levelname)s | %(message)s', level=logging.DEBUG)
mu = 4
sigma = 2
upper = mu + sigma

# For log level
alpha = 1
beta = 1.5
levels = ['debug', 'info', 'warn', 'error', 'critical']

flip = 1

while True:
    temp = random.gauss(mu, sigma)
    not_multiline = sigma <= temp <= upper

    not_multiline = flip
    flip ^= 1

    temp = round(random.weibullvariate(alpha, beta))
    if temp >= len(levels):
        temp = len(levels) -1

    level = levels[temp]
    match level:
        case 'debug':
            if not_multiline:
                logging.debug('This is a debug log that shows a log that can be ignored."')
            else:
                logging.debug('This is a \ndebug log that \nshows a log that \ncan be ignored."')
        case 'info':
            if not_multiline:
                logging.info('This is less important than debug log and is often used to provide context in the current task.')
            else:
                logging.info('This is less \nimportant than debug log \nand is often used to provide context \nin the current task.')
        case 'warn':
            if not_multiline:
                logging.warning('A warning that should be ignored is usually at this level and should be actionable.')
            else:
                logging.warning('A warning \nthat should be ignored is \nusually at this level and should be actionable.')
        case 'error':
            if not_multiline:
                logging.error('An error is usually an exception that has been caught and not handled.')
            else:
                logging.error('An error is \nusually an exception that \nhas been caught and not handled.')
        case 'critical':
            if not_multiline:
                logging.critical('A critical error is usually an exception that has not been caught or handled.')
            else:
                logging.critical('A critical error is \nusually an exception that \nhas not been caught or handled.')
    sleep(random.uniform(0, 5))
EOF
sudo mv random-logger.py /usr/bin/random-logger.py
sudo chmod +x /usr/bin/random-logger.py
