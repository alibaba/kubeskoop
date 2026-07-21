FROM docker.io/library/golang:1.25.12-alpine3.24
ARG GOPROXY
RUN apk add clang21=21.1.8-r3 llvm21=21.1.8-r1 linux-headers git make bash && \
    ln -s /usr/bin/clang-21 /usr/local/bin/clang && \
    ln -s /usr/bin/llvm21-strip /usr/local/bin/llvm-strip
RUN if [ -n "${GOPROXY}" ]; then go env -w GOPROXY="${GOPROXY}"; fi && \
    go install github.com/cilium/ebpf/cmd/bpf2go@latest
