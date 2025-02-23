FROM alpine:latest

ENV LANG=C.UTF-8 \
    UDS="/app/qbt.sock" \
    PORT=8086 \
    PASSWORD="fnosnb"

WORKDIR /app

COPY fnos-qb-proxy_linux-amd64 /usr/local/bin/fnos-qb-proxy

RUN chmod +x /usr/local/bin/fnos-qb-proxy && \
    echo "https://mirrors.tuna.tsinghua.edu.cn/alpine/latest-stable/main" > /etc/apk/repositories && \ 
    echo "https://mirrors.tuna.tsinghua.edu.cn/alpine/latest-stable/community" >> /etc/apk/repositories && \ 
    apk add --no-cache bash

CMD ["sh", "-c", "fnos-qb-proxy --uds $UDS --password $PASSWORD --port $PORT"]
