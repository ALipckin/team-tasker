FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/api /app/api
COPY internal/database/migrations /app/internal/database/migrations

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/app/api"]
