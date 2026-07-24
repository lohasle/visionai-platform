FROM alpine:3.22.1

RUN addgroup -S -g 10001 trainer \
    && adduser -S -D -H -u 10001 -G trainer trainer

COPY --chmod=0555 deploy/trainer-smoke/run.sh /usr/local/bin/visionai-trainer

USER 10001:10001
ENTRYPOINT ["/usr/local/bin/visionai-trainer"]
