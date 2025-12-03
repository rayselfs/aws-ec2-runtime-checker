FROM --platform=$BUILDPLATFORM golang:1.24 AS base

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -o /app/main ./cmd/main.go

FROM gcr.io/distroless/static-debian12 AS final

USER nonroot:nonroot

WORKDIR /app

COPY --from=base /app/main /app

ENTRYPOINT ["/app/main"]
