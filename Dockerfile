FROM docker.io/library/golang:1.20.5-alpine AS build
# --build-arg GOPROXY=https://goproxy.cn,direct
ARG GOPROXY
# --build-arg ALPINE_MIRROR=mirrors.aliyun.com
ARG ALPINE_MIRROR

RUN if [ ! -z "$ALPINE_MIRROR" ]; then sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories; fi && \
    apk add gcc g++ linux-headers git make bash && \
    go env -w GOPROXY=$GOPROXY

WORKDIR /go/src/github.com/alibaba/kubeskoop/
ADD . /go/src/github.com/alibaba/kubeskoop/
RUN mkdir -p bin && make all

FROM docker.io/library/node:20.9.0-alpine as build-ui
WORKDIR /webconsole
ADD ./webui /webconsole
RUN yarn install && yarn build

FROM docker.io/library/alpine

RUN apk add --no-cache \
    iproute2 \
    ipset \
    iptables \
    ipvsadm \
    jq \
    strace \
    tcpdump \
    curl \
    bash

COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/inspector /bin/inspector
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/pod-collector /bin/pod-collector
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/controller /bin/controller
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/skoop /bin/skoop
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/btfhack /bin/btfhack
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/webconsole /bin/webconsole

COPY --from=build-ui /webconsole/build /var/www

COPY tools/scripts/* /bin/
COPY deploy/resource/kubeskoop-exporter-dashboard.json /etc/
