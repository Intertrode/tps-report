# ---- builder ----
FROM golang:1.26 AS builder
WORKDIR /app
COPY src/go.mod src/go.sum ./
RUN go mod download
COPY src/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /tps-report .

# ---- final ----
FROM gcr.io/distroless/static-debian12
COPY --from=builder /tps-report /tps-report
EXPOSE 8080
ENTRYPOINT ["/tps-report"]