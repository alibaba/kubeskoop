FROM kubeskoop/ci-builder:latest AS build

WORKDIR /go/src/github.com/alibaba/kubeskoop/
RUN go env -w GOMODCACHE=/root/.cache/go-build
COPY go.mod go.sum /go/src/github.com/alibaba/kubeskoop/
RUN --mount=type=cache,target=/root/.cache/go-build go mod download

ADD . /go/src/github.com/alibaba/kubeskoop/
RUN --mount=type=cache,target=/root/.cache/go-build mkdir -p bin && make generate-bpf && make all

FROM --platform=linux/amd64 docker.io/library/node:20.9.0-alpine as build-ui
WORKDIR /webconsole
ADD ./webui /webconsole
RUN yarn install && yarn build

FROM docker.io/library/alpine:3.19 as base

ARG ALPINE_MIRROR
ENV ALPINE_MIRROR=$ALPINE_MIRROR

RUN if [ ! -z "$ALPINE_MIRROR" ]; then sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories; fi && \
    apk add --no-cache \
    iproute2 \
    ipset \
    iptables \
    iptables-legacy \
    ipvsadm \
    jq \
    strace \
    tcpdump \
    curl \
    bash && \
    rm -rf /var/cache/apk/*

FROM base as agent
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/inspector /bin/inspector
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/pod-collector /bin/pod-collector
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/btfhack /bin/btfhack

FROM base as controller
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/controller /bin/controller
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/skoop /bin/skoop
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/webconsole /bin/webconsole
COPY --from=build-ui /webconsole/build /var/www

COPY tools/scripts/* /bin/
COPY deploy/resource/kubeskoop-exporter-dashboard.json /etc/
