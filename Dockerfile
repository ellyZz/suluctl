FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /work
COPY suluctl /usr/local/bin/suluctl
ENTRYPOINT ["suluctl"]
