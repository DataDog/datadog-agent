#!/bin/bash
cat > /usr/bin/random-logger.py << EOF
#!/usr/bin/env python3

import logging
import random
from time import sleep

logging.basicConfig(format="%(asctime)s | %(levelname)s | %(message)s", level=logging.DEBUG)

while True:
    logging.info("This is less important than debug log and is often used to provide context in the current task.")
    sleep(random.uniform(0, 5))
EOF
sudo chmod +x /usr/bin/random-logger.py
