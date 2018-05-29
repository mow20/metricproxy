# Docker multistage builds are powerful but they run sequentially
# through the dockerfile and you can only specify when to stop on 
# a target stage.  The order of the image stages matter in this Dockerfile.
ARG GO_VERSION=1.10

### BASE ###
# This stage is the basis for building the proxy and is where build dependencies
# should be installed into the build image.
FROM golang:${GO_VERSION}-stretch as metricproxy-base
RUN apt-get -y update && apt-get -y install file
RUN go get -u github.com/signalfx/gobuild
RUN go get -u github.com/alecthomas/gometalinter
RUN gometalinter --install
COPY . /go/src/github.com/signalfx/metricproxy
WORKDIR /go/src/github.com/signalfx/metricproxy
### /BASE ###

### BINARY ###
# This stage is where the proxy is actually built
FROM metricproxy-base as metricproxy-binary
ARG GOOS=linux
ARG GOARCH=amd64
ARG CGO_ENABLED=0
RUN go build -v -installsuffix . -ldflags="-s -w"
### /BINARY ###

### METRICPROXY IMAGE ###
# This stage is the actual metricproxy image that is distributed.
# It copies the metricproxy binary out of the BINARY stage.  This stage is 
# exported as the final image durring the FINAL stage.  This allows developers
# to build a dev image without running gobuild after running the GOBUILD stage.
FROM scratch as metricproxy-image
MAINTAINER Matthew Pound <mwp@signalfx.com>

COPY ca-bundle.crt /etc/pki/tls/certs/ca-bundle.crt
COPY --from=metricproxy-binary /go/src/github.com/signalfx/metricproxy/metricproxy /metricproxy

VOLUME /var/log/sfproxy
VOLUME /var/config/sfproxy

CMD ["/metricproxy", "-configfile", "/var/config/sfproxy/sfdbproxy.conf"]
### /METRICPROXY IMAGE ###

### GOBUILD ###
# Running this tage after the metricproxy-binary and metricproxy-image stages
# allows the metricproxy-binary and metricproxy-image stages to be buit for 
# dev/test purposes without running gobuild everytime
FROM metricproxy-base as metricproxy-gobuild
RUN /go/src/github.com/signalfx/metricproxy/scripts/gobuild.sh
### /GOBUILD ###

### FINAL###
# This allows metricproxy-image stage to be exported after running the 
# metricproxy-gobuild stage
FROM metricproxy-image as metricproxy-final
### /FINAL###
