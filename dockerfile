# FROM centos:8
FROM golang:latest
MAINTAINER SmartBrave <SmartBraveCoder@gmail.com>

#build command: docker build -t gobog
#run command: docker run -v $(pwd)/../blog:/go/src/github.com/SmartBrave/blog gobog
ARG  CERT_PATH=..\\/blog\\/cert
ARG  SOURCE_PATH=..\\/blog\\/source
ARG  IMAGE_PATH=..\\/..\\/blog\\/source\\/image

# RUN yum -y install wget
# RUN wget https://golang.org/dl/go1.16.7.linux-amd64.tar.gz \
    # && tar xvf go1.16.7.linux-amd64.tar.gz \
    # && cp -r go /usr/local/go

COPY . $GOPATH/src/github.com/SmartBrave/gobog
WORKDIR $GOPATH/src/github.com/SmartBrave/gobog

# RUN /usr/local/go/bin/go build src/main.go
RUN go build src/main.go
RUN sed -i "s/\${YOUR_CERT_PATH}/$CERT_PATH/g" conf/config.toml
RUN sed -i "s/\${YOUR_SOURCE_PATH}/$SOURCE_PATH/g" conf/config.toml
RUN sed -i "s/\${IMAGE_PATH}/$IMAGE_PATH/g" script/export.sh

# CMD ./main
CMD pwd && ls ../*
