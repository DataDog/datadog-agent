FROM python:2.7-alpine3.6

# datadog-py has no release with UDS support yet, using a commit hash
RUN pip install --no-cache-dir https://github.com/DataDog/datadogpy/archive/8b19b0b6e2d5e898dc05800f8257430b68156471.zip

COPY sender.py /sender.py

CMD [ "python", "/sender.py" ]
