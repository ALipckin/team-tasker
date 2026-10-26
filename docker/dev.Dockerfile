FROM golang:1.26-bookworm

WORKDIR /workspace

RUN \
    go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.4 && \
    go install github.com/air-verse/air@v1.62.0 && \
	go install github.com/go-delve/delve/cmd/dlv@v1.26.1

COPY go.mod go.sum ./
RUN go mod download

COPY . .

CMD ["air", "-c", "./docker/.air.toml"]
