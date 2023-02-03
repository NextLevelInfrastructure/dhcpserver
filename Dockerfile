# Dockerfile - build Next Level dhcpserver container

ARG GO_VERSION=1.19.3

######################################################
######################## STAGE 1: build the executable
########################

FROM golang:${GO_VERSION}-alpine AS builder

# required for go mod
RUN apk --no-cache add git curl ca-certificates

WORKDIR /src

ARG VERBOSE

# Cause the Docker cache to be invalidated when git dhcpserver changes
# In particular, when go.mod for the main module changes, we'll start from here.
ADD https://api.github.com/repos/NextLevelInfrastructure/dhcpserver/git/refs/heads/main dhcpserver-version.json  

# Cause the Docker cache to be invalidated when git coredhcp changes
ADD https://api.github.com/repos/NextLevelInfrastructure/coredhcp/git/refs/heads/master coredhcp-version.json

RUN git clone https://github.com/NextLevelInfrastructure/dhcpserver.git
RUN git clone https://github.com/NextLevelInfrastructure/coredhcp.git

RUN go mod edit -replace github.com/coredhcp/coredhcp=../coredhcp dhcpserver/go.mod
RUN cd /src/dhcpserver && go mod tidy

# Run unit tests and build the executable
RUN cd /src/coredhcp   && CGO_ENABLED=0 go test $VERBOSE -timeout 30s ./...
RUN cd /src/dhcpserver && CGO_ENABLED=0 go test $VERBOSE -timeout 30s ./...
RUN cd /src/dhcpserver && CGO_ENABLED=0 go build -installsuffix 'static' -o /dhcpserver ./cmd

####################################################
######################## STAGE 2: create the runtime
########################

FROM scratch

MAINTAINER daniel.dulitz@nextlevel.net

EXPOSE 67/udp
EXPOSE 547/udp

USER 10000

COPY --from=builder /dhcpserver /dhcpserver

WORKDIR /configs/dhcpserver

ENTRYPOINT [ "/dhcpserver" ]
CMD [ "--conf=dhcpserver.yml" ]
