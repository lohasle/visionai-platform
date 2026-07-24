FROM golang:1.24.8-alpine3.22 AS builder

ARG MINIO_VERSION=RELEASE.2025-10-15T17-29-55Z
RUN CGO_ENABLED=0 go install "github.com/minio/minio@${MINIO_VERSION}"

FROM alpine:3.22.1
RUN apk add --no-cache su-exec \
    && addgroup -S minio \
    && adduser -S -G minio minio \
    && mkdir -p /data
COPY --from=builder /go/bin/minio /usr/local/bin/minio
COPY deploy/minio/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod 0755 /usr/local/bin/entrypoint.sh
EXPOSE 9000 9001
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
