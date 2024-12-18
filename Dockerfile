FROM golang:1.22-alpine3.20 AS builder

WORKDIR /operator

RUN apk add --update --no-cache bash curl git make

COPY ./go.* ./
COPY ./Makefile ./
RUN make controller-gen

COPY ./main.go ./
COPY ./apis/ ./apis/
COPY ./internal/ ./internal/
COPY ./controllers/ ./controllers/
COPY ./hack/ ./hack/

RUN make build
RUN cp ./bin/iofog-operator /bin

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
WORKDIR /

RUN microdnf install -y shadow-utils && \
    microdnf clean all
RUN useradd --uid 10000 runner


COPY LICENSE /licenses/LICENSE
COPY --from=builder /bin/iofog-operator /bin/
LABEL org.opencontainers.image.description=operator
LABEL org.opencontainers.image.source=https://github.com/datasance/iofog-operator
LABEL org.opencontainers.image.licenses=EPL2.0

USER 10000

ENTRYPOINT ["/bin/iofog-operator", "--enable-leader-election"]
