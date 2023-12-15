FROM python:3.11-alpine

RUN pip install --no-cache-dir datadog
COPY sender.py /sender.py

CMD [ "python", "/sender.py" ]
