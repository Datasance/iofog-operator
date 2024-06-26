FROM golang:1.21.5-alpine3.19 as builder

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

FROM alpine:3.19
WORKDIR /
COPY --from=builder /bin/iofog-operator /bin/
LABEL org.opencontainers.image.description operator
LABEL org.opencontainers.image.source=https://github.com/datasance/iofog-operator
LABEL org.opencontainers.image.licenses=EPL2.0
ENTRYPOINT ["/bin/iofog-operator", "--enable-leader-election"]
