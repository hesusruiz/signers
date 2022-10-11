FROM golang:1.18 as builder

WORKDIR /usr/src

# Download quorum
RUN git clone https://github.com/ConsenSys/quorum.git

# Download signers
RUN git clone https://github.com/hesusruiz/signers.git

WORKDIR /usr/src/signers
RUN go mod tidy & go mod download && go mod verify

RUN go build -v -o signers .

# Now copy it into a minimal distroless base image
FROM gcr.io/distroless/base-debian11
WORKDIR /
COPY --from=builder /usr/src/signers/signers /
ENTRYPOINT [ "/signers" ]
