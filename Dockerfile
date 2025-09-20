FROM golang:1.25.1-bookworm AS build

WORKDIR /build

COPY llvm-snapshot.gpg.key .

RUN apt-get update && \
    apt-get -y --no-install-recommends install ca-certificates gnupg && \
    apt-key add llvm-snapshot.gpg.key && \
    rm llvm-snapshot.gpg.key && \
    apt-get remove -y gnupg && \
    apt-get autoremove -y && \
    rm -rf /var/lib/apt/lists/*

COPY llvm.list /etc/apt/sources.list.d

RUN apt-get update && \
    apt-get -y --no-install-recommends install \
    make git \
    bpftool \
    libbpf-dev \
    clang-format \
    clang-20 llvm-20 && \
    rm -rf /var/lib/apt/lists/*

ARG VERSION
ARG SHA1
ARG BUILDDATE

COPY . .

RUN go generate ./...

RUN LDFLAG_LOCATION=github.com/eminaktas/oom-tracer/pkg/version; \
    CGO_ENABLED=0 go build -o oom-tracer -a -ldflags "-X ${LDFLAG_LOCATION}.version=${VERSION} -X ${LDFLAG_LOCATION}.buildDate=${BUILDDATE} -X ${LDFLAG_LOCATION}.gitsha1=${SHA1}" ./cmd

FROM alpine:latest

WORKDIR /oom-tracer

COPY --from=build /build/oom-tracer ./

ENTRYPOINT [ "/oom-tracer/oom-tracer" ]
