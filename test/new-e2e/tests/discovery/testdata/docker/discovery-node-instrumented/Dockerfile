FROM node:20-slim

COPY package.json server.js /app/
WORKDIR /app

RUN npm install

ENV PORT=8085

CMD ["node", "server.js"]
