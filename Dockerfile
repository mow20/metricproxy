FROM ubuntu
MAINTAINER Matthew Pound <mwp@signalfx.com>

COPY ca-bundle.crt /etc/pki/tls/certs/ca-bundle.crt
COPY metricproxy /metricproxy

RUN apt-get update
RUN apt-get install -y wget curl
RUN wget https://dl.google.com/go/go1.11.1.linux-amd64.tar.gz  && \
tar -xvf go1.11.1.linux-amd64.tar.gz && \
mv go /usr/local && \
export GOROOT=/usr/local/go && \
export GOPATH=$HOME && \
export PATH=$GOPATH/bin:$GOROOT/bin:$PATH

RUN curl -s \https://raw.githubusercontent.com/signalfx/metricproxy/master/install.sh\ | sh

VOLUME /var/log/sfproxy
VOLUME /var/config/sfproxy

EXPOSE 2003

CMD ["/bin/bash","/etc/init.d/metricproxy", "start"]
