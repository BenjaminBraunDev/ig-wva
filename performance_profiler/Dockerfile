# Use a minimal base image suitable for Go binaries
FROM gcr.io/distroless/base-debian11 AS final

# Set the working directory in the container
WORKDIR /app

# Copy the compiled Go binary from the blaze-bin directory (relative to google3 root)
# into the container.
# The docker build command in the README uses '.' (google3 root) as the build context.
COPY performance_profiler/cmd/server/main /app/server

# Expose the default port the application listens on
EXPOSE 8080

# Set the entrypoint for the container to run the server binary.
# Flags like --port, --spanner_db, or --use_mock_data can be passed
# when running the container (e.g., docker run myimage --port=8080 --use_mock_data).
ENTRYPOINT ["/app/server"]