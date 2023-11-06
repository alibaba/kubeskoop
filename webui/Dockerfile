FROM docker.io/library/golang:1.20.5-alpine as builder
ARG GOPROXY

WORKDIR /go/src/github.com/kubeskoop/webconsole
ADD . /go/src/github.com/kubeskoop/webconsole

RUN mkdir -p bin && \
    go env -w GOPROXY=$GOPROXY && \
    go build -o bin/webconsole .

FROM docker.io/library/node:20.9.0-alpine as build-ui
WORKDIR /webconsole
ADD . /webconsole
RUN yarn install && yarn build


FROM docker.io/library/alpine
COPY --from=builder /go/src/github.com/kubeskoop/webconsole/build/webconsole /usr/local/bin/webconsole
COPY --from=build-ui /webconsole/build /var/www
ENTRYPOINT [ "webconsole" ]
