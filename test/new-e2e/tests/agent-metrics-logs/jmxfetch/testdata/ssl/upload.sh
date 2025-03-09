#!/bin/sh

tar czvf secrets.tar.gz */*store
base64 < secrets.tar.gz > secrets.tar.gz.base64
aws secretsmanager create-secret --name "agent-e2e-jmxfetch-test-certs-$(date +%Y%m%d)" --secret-binary fileb://secrets.tar.gz.base64
