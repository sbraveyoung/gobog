FROM centos:8
MAINTAINER SmartBrave <SmartBraveCoder@gmail.com>

#build command: docker build -t IMAGE_NAME --build-arg CERT_PATH=xxx --build-arg SOURCE_PATH=xxx --build-arg IMAGE_PATH=xxx .
ARG  CERT_PATH=..\/blog\/cert
ARG  SOURCE_PATH=..\/blog\/source
ARG  IMAGE_PATH=..\/..\/blog\/source\/image

RUN yum -y install wget
RUN wget https://golang.org/dl/go1.16.7.linux-amd64.tar.gz \
    && tar xvf go1.16.7.linux-amd64.tar.gz \
    && cp -r go /usr/local/go

COPY . $GOPATH/src/github.com/SmartBrave/gobog
WORKDIR $GOPATH/src/github.com/SmartBrave/gobog

RUN go build src/main.go
RUN cert_path=$CERT_PATH && sed -i "s/\${YOUR_CERT_PATH}/$cert_path/g" conf/config.toml
# RUN sed -i "s/\${YOUR_SOURCE_PATH}/\$SOURCE_PATH/g" conf/config.toml
# RUN sed -i "s/\${IMAGE_PATH}/\$IMAGE_PATH/g" script/export.sh

# CMD /usr/bin/bash
# CMD ./main
CMD pwd && echo "-----------------------" && ls * && echo "--------------------------------" && cat conf/config.toml && echo "--------------------------------------" && cat script/export.sh
