# syntax=docker/dockerfile:1
# Multi-stage build for the e-agora backend. The binary is self-contained:
# migrations and seed data are embedded (go:embed), so the runtime image needs
# no extra files. distroless/static ships CA certs for the HTTPS calls to
# Wikidata/Wikipedia during seeding.

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/eagora ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/eagora /eagora
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/eagora"]
