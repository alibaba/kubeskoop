FROM docker.io/library/golang:1.22.1-alpine
RUN apk add clang=17.0.5-r0 llvm=17.0.5-r0 linux-headers git make bash
RUN go install github.com/cilium/ebpf/cmd/bpf2go@latest
