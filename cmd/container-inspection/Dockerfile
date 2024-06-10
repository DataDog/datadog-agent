FROM golang:1.21-alpine as build

WORKDIR /app/src

COPY . .

RUN go build -o /app/inspect ./cmd/container-inspection/inspect
RUN go build -o /app/entry ./cmd/container-inspection/entry

FROM alpine

COPY --from=build /app/inspect /inspect
COPY --from=build /app/entry  /dd-source/entry

CMD [ "/inspect" ]
