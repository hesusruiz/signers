FROM ubuntu:20.04

WORKDIR /usr/local/bin

COPY signers .

CMD ["./signers"]
