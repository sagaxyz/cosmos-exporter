# Build container
FROM golang:1.21 AS builder

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o cosmos-exporter

# Runtime container
FROM alpine:3.19.1

ENV BECH_ACCOUNT_PREFIX saga
ENV DENOM KEK
ENV STATS_ENDPOINT localhost:9090
ENV LISTEN_ADDRESS localhost:9300

WORKDIR /app

COPY --from=builder /app/cosmos-exporter .

EXPOSE 9090

CMD ./cosmos-exporter --bech-prefix ${BECH_ACCOUNT_PREFIX} --denom ${DENOM} --node ${STATS_ENDPOINT} --listen-address=$LISTEN_ADDRESS