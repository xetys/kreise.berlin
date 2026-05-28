# Backend image: Go 1.25 builder, distroless-static runtime.
# Migrations are go:embed-ed into the binary, so no extra files at runtime.
#
# Build context MUST be the repo root (go.mod lives there). From the Makefile:
#   docker buildx build --platform linux/amd64 -f backend/Dockerfile -t ... .

FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache modules first. go.mod + go.sum live at the repo root.
COPY go.mod go.sum ./
RUN go mod download

# Pull in only the backend tree so the frontend doesn't bloat the build context
# (a .dockerignore at the repo root excludes node_modules / .next anyway).
COPY backend/ ./backend/

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -trimpath -ldflags="-s -w" -o /out/server          ./backend/cmd/server
RUN go build -trimpath -ldflags="-s -w" -o /out/bootstrap-admin ./backend/cmd/bootstrap-admin

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/server          /app/server
COPY --from=build /out/bootstrap-admin /app/bootstrap-admin
EXPOSE 8080
USER nonroot:nonroot
# Default entrypoint is the HTTP server. Helm Jobs override with
#   command: ["/app/bootstrap-admin"]
# to run the one-shot first-admin seeder.
ENTRYPOINT ["/app/server"]
