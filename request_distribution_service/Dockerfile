# Use a minimal base image for the Go binary
FROM gcr.io/distroless/static-debian11 AS release

WORKDIR /app

# Copy the compiled Go binary from the Blaze build output
# The path inside blaze-bin will match the path of the go_binary target
COPY request_distribution_service_go/cmd/server/main /app/server

# Set the entrypoint for the container
ENTRYPOINT ["/app/server"]

# Default port, can be overridden by --port flag to the binary
EXPOSE 8081
