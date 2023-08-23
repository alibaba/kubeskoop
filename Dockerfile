ARG CILIUM_LLVM_IMAGE=quay.io/cilium/cilium-llvm:547db7ec9a750b8f888a506709adb41f135b952e@sha256:4d6fa0aede3556c5fb5a9c71bc6b9585475ac9b1064f516d4c45c8fb691c9d9e
ARG CILIUM_BPFTOOL_IMAGE=quay.io/cilium/cilium-bpftool:78448c1a37ff2b790d5e25c3d8b8ec3e96e6405f@sha256:99a9453a921a8de99899ef82e0822f0c03f65d97005c064e231c06247ad8597d
ARG CILIUM_IPROUTE2_IMAGE=quay.io/cilium/cilium-iproute2:3570d58349efb2d6b0342369a836998c93afd291@sha256:1abcd7a5d2117190ab2690a163ee9cd135bc9e4cf8a4df662a8f993044c79342
# --build-arg GOPROXY=https://goproxy.cn,direct
ARG GOPROXY
# --build-arg ALPINE_MIRROR=mirrors.aliyun.com
ARG ALPINE_MIRROR

FROM --platform=$TARGETPLATFORM ${CILIUM_LLVM_IMAGE} as llvm-dist
FROM --platform=$TARGETPLATFORM ${CILIUM_BPFTOOL_IMAGE} as bpftool-dist
FROM --platform=$TARGETPLATFORM ${CILIUM_IPROUTE2_IMAGE} as iproute2-dist

FROM docker.io/library/golang:1.19.4-alpine AS build
RUN if [ ! -z "$ALPINE_MIRROR" ]; then sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories; fi && \
    apk add gcc g++ linux-headers git make bash && \
    go env -w GOPROXY=$GOPROXY

WORKDIR /go/src/github.com/alibaba/kubeskoop/
ADD . /go/src/github.com/alibaba/kubeskoop/
RUN mkdir -p bin && make all

FROM --platform=$TARGETPLATFORM ubuntu:20.04
RUN apt-get update && apt-get install -y kmod libelf1 libmnl0 iptables nftables kmod curl ipset bash ethtool bridge-utils socat grep findutils jq conntrack iputils-ping ipvsadm iproute2 strace tcpdump && \
    apt-get purge --auto-remove && apt-get clean && rm -rf /var/lib/apt/lists/*

COPY --from=llvm-dist /usr/local/bin/clang /usr/local/bin/llc /usr/local/bin/llvm-objcopy /bin/
COPY --from=bpftool-dist /usr/local /usr/local
COPY --from=iproute2-dist /usr/local /usr/local
COPY --from=iproute2-dist /usr/lib/libbpf* /usr/lib/
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/inspector /bin/inspector
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/pod-collector /bin/pod-collector
COPY --from=build /go/src/github.com/alibaba/kubeskoop/bin/skoop /bin/skoop
COPY tools/scripts/* /bin/
COPY bpf /var/lib/bpf
COPY deploy/resource/kubeskoop-exporter-dashboard.json /etc/
