FROM docker.io/library/golang:1.18.4-alpine AS build
ARG GOPROXY # --build-arg GOPROXY=https://goproxy.cn,direct
ARG ALPINE_MIRROR # --build-arg ALPINE_MIRROR=mirrors.aliyun.com

RUN if [ ! -z "$ALPINE_MIRROR" ]; then sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories; fi && \
    apk add gcc g++ linux-headers && \
    go env -w GOPROXY=$GOPROXY

WORKDIR /go/src/github.com/alibaba/kubeskoop/
ADD . /go/src/github.com/alibaba/kubeskoop/
RUN mkdir bin && cd cmd/collector && go build -o /go/src/github.com/alibaba/kubeskoop/bin/pod-collector
RUN cd /go/src/github.com/alibaba/kubeskoop/cmd/exporter && go build -o /go/src/github.com/alibaba/kubeskoop/bin/inspector
RUN cd /go/src/github.com/alibaba/kubeskoop/cmd/skoop && go build -o /go/src/github.com/alibaba/kubeskoop/bin/skoop


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
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/skoop /bin/skoop
