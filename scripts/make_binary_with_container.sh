#!/bin/bash
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
tag=${1:-$USER}-binary
echo $tag
echo $2

docker build --target metricproxy-binary \
             --build-arg GOOS=${2:-linux}\
             -t quay.io/signalfx/metricproxy:$tag $SCRIPT_DIR/..
cid=$(docker create quay.io/signalfx/metricproxy:$tag true)
trap "docker rm -f $cid; docker rmi quay.io/signalfx/metricproxy:$tag" EXIT
docker cp $cid:/go/src/github.com/signalfx/metricproxy/metricproxy $SCRIPT_DIR/../metricproxy

