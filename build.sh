#!/bin/bash
set -e

SRCDIR=`pwd`
BUILDDIR=`pwd`/build

if [ -d "${BUILDDIR}" ]; then
    rm -rf ${BUILDDIR}
fi
mkdir -p ${BUILDDIR} 2>/dev/null
cd ${BUILDDIR}
echo "Cloning coredns repo..."
git clone https://github.com/coredns/coredns.git
cd coredns
git checkout v1.12.0

echo "nfd:github.com/TxnLab/nfd-coredns" >> ./plugin.cfg
echo 'go 1.23.3

      toolchain go1.23.3

      use (
      	.
      	../..
      )
' >> ./go.work

echo "Building..."
# bypass make which does make gen first - but then which uses go get that doesn't work with modules
# so do generate first which then satisfies the prereqs that cause go get to be run.
go generate coredns.go
mkdir -p out/linux/amd64  && make coredns BINARY=out/linux/amd64/coredns SYSTEM="GOOS=linux GOARCH=amd64" CHECKS="" BUILDOPTS=""
#mkdir -p out/linux/arm64  && make coredns BINARY=out/linux/arm64/coredns SYSTEM="GOOS=linux GOARCH=arm64" CHECKS="" BUILDOPTS=""

cd ${SRCDIR}
echo building docker image
docker build --platform linux/amd64 . -t REDACTED_REGISTRY/nfddns:latest
docker push REDACTED_REGISTRY/nfddns:latest
#rm -rf ${BUILDDIR}
