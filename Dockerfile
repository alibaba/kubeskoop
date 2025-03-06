FROM --platform=$TARGETPLATFORM kubeskoop/ci-builder:go124clang191 AS bpf-build
WORKDIR /go/src/github.com/alibaba/kubeskoop/
RUN go env -w GOMODCACHE=/root/.cache/go-build
COPY go.mod go.sum /go/src/github.com/alibaba/kubeskoop/
RUN --mount=type=cache,target=/root/.cache/go-build go mod download
ADD . /go/src/github.com/alibaba/kubeskoop/
RUN --mount=type=cache,target=/root/.cache/go-build make generate-bpf

FROM --platform=$BUILDPLATFORM kubeskoop/ci-builder:go124clang191 AS cross-build
WORKDIR /go/src/github.com/alibaba/kubeskoop/
RUN go env -w GOMODCACHE=/root/.cache/go-build
COPY go.mod go.sum /go/src/github.com/alibaba/kubeskoop/
RUN --mount=type=cache,target=/root/.cache/go-build go mod download
COPY --from=bpf-build /go/src/github.com/alibaba/kubeskoop/ /go/src/github.com/alibaba/kubeskoop/
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build mkdir -p bin && export GOARCH=$TARGETARCH && make all

FROM --platform=$BUILDPLATFORM docker.io/library/node:20.9.0-alpine AS build-ui
WORKDIR /webconsole
ADD ./webui /webconsole
RUN yarn install && yarn build

FROM --platform=$TARGETPLATFORM docker.io/library/alpine:3.19 AS base

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

FROM base AS agent
COPY --from=cross-build /go/src/github.com/alibaba/kubeskoop/bin/inspector /bin/inspector
COPY --from=cross-build /go/src/github.com/alibaba/kubeskoop/bin/pod-collector /bin/pod-collector
COPY --from=cross-build /go/src/github.com/alibaba/kubeskoop/bin/btfhack /bin/btfhack

FROM base AS controller
COPY --from=cross-build /go/src/github.com/alibaba/kubeskoop/bin/controller /bin/controller
COPY --from=cross-build /go/src/github.com/alibaba/kubeskoop/bin/skoop /bin/skoop
COPY --from=cross-build /go/src/github.com/alibaba/kubeskoop/bin/webconsole /bin/webconsole
COPY --from=build-ui /webconsole/build /var/www

COPY tools/scripts/* /bin/
COPY deploy/resource/kubeskoop-exporter-pods-dashboard.json deploy/resource/kubeskoop-exporter-nodes-dashboard.json /etc/
