# syntax=docker/dockerfile:1

# ---------- build stage ----------
FROM golang:1.25 AS builder
WORKDIR /src

# Dependencies first so this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api     ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
 && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/seed    ./cmd/seed

# ---------- runtime stage ----------
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

# One image, three binaries: the API (default), plus migrate and seed which
# the deployment runs as one-shot commands. Distroless has no shell or `go`,
# so the migration runner is compiled in and the SQL files are copied next to it.
COPY --from=builder /out/api /out/migrate /out/seed ./
COPY db/migrations ./db/migrations

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["./api"]
