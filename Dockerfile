# Build stage
FROM golang:1.18.3-alpine3.16 as go-builder
ARG GOPROXY=goproxy.cn

ENV GOPROXY=https://${GOPROXY},direct
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
RUN apk add --no-cache make bash git tzdata

WORKDIR /data

COPY go.mod go.sum ./
RUN go mod download -x
COPY . .
RUN ls -rlt ./ && make build

# 运行阶段
# 需要go环境
FROM golang:1.18.3-alpine3.16
LABEL maintainer="goprobe@gotomicro.com"
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
USER root
ARG APP=goprobe
ENV APP=${APP}
ENV WORKDIR=/data
ENV GOPROXY=https://goproxy.cn,direct

COPY ./scripts/flamegraph.pl /bin/flamegraph.pl
# install graphivz,perl and set timeZone to Asia/Shanghai
RUN apk add --no-cache graphviz
RUN apk add perl
RUN chmod a+x /bin/flamegraph.pl
RUN apk add --no-cache tzdata bash

COPY --from=go-builder /data/bin/${APP} ${WORKDIR}/bin/
COPY --from=go-builder /data/config ${WORKDIR}/config

ENV TZ="Asia/Shanghai"
WORKDIR ${WORKDIR}

# http
EXPOSE 9001
# 预留
EXPOSE 9002
# govern
EXPOSE 9003
#ENV EGO_DEBUG=true
#ENTRYPOINT ["sh","-c","sleep 3000s"," && ","/data/goprobe","--config=config/local.toml"]
#ENTRYPOINT ["/data/goprobe","--config=config/local.toml"]
CMD ["sh", "-c", "./bin/goprobe"]