FROM docker.io/library/golang:1.24.1-alpine
RUN apk add clang=19.1.4-r0 llvm=19.1.4-r0 linux-headers git make bash
RUN go install github.com/cilium/ebpf/cmd/bpf2go@latest
