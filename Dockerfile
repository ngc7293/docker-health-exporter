FROM golang:1.24.1 AS builder

WORKDIR /build

COPY go.mod go.sum /build
RUN go mod download

COPY main.go       /build
RUN CGO_ENABLED=0 GOOS=linux go build -o docker-health-exporter .

FROM scratch AS runtime

COPY --from=builder /build/docker-health-exporter /docker-health-exporter

EXPOSE 8080
HEALTHCHECK --start-period=0s --interval=10s --retries=1 CMD [ "/docker-health-exporter", "--health-check" ]
ENTRYPOINT [ "/docker-health-exporter" ]