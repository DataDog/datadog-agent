FROM node:20-slim

RUN npm install -g json-server

COPY db.json /app/db.json
WORKDIR /app

ENV PORT=8084

ENTRYPOINT ["sh", "-c", "npx json-server --host 0.0.0.0 --port $PORT db.json"]
