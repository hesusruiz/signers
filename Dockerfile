FROM golang:1.18 as builder

WORKDIR /usr/src

# Download quorum
RUN git clone https://github.com/ConsenSys/quorum.git

# Download signers
RUN git clone https://github.com/hesusruiz/signers.git

WORKDIR /usr/src/signers
RUN go mod tidy
RUN go build -o signers .

FROM ubuntu:20.04

WORKDIR /usr/local/bin

COPY --from=builder /usr/src/signers/signers /usr/local/bin/

ENTRYPOINT [ "signers" ]
