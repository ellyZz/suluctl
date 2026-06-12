FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY suluctl /usr/local/bin/suluctl
ENTRYPOINT ["suluctl"]
