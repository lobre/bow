FROM node:alpine as node
FROM golang:alpine as builder

# install build deps
RUN apk add build-base

# build application
COPY . /go/src/app
WORKDIR /go/src/app
RUN --mount=type=cache,target=/root/.cache/go-build go generate && go install

# final image
FROM alpine
[[- if .WithDB ]]
RUN apk add --no-cache sqlite
[[- end ]]
COPY --from=builder /go/bin/[[ .Binary ]] /usr/local/bin/
EXPOSE 8080
VOLUME /root
ENTRYPOINT [ "[[ .Binary ]]" ]
[[- if .WithDB ]]
CMD [ "-dsn", "/root/[[ .Binary ]].db" ]
[[- end ]]
