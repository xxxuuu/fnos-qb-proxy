FROM golang:1.22.0 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 go build -o fnos-qb-proxy
RUN chmod +x fnos-qb-proxy

FROM alpine:latest

ENV LANG=C.UTF-8 \
    PORT=8086 \
    PASSWORD="fnosnb"

WORKDIR /app
COPY --from=builder /app/fnos-qb-proxy /usr/local/bin/fnos-qb-proxy

ENTRYPOINT ["sh", "-c", "exec fnos-qb-proxy --password \"$PASSWORD\" --port \"$PORT\" \"$@\"", "--"]
