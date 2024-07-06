# Use the official Go image from the Docker Hub
FROM golang:1.17

# Set the working directory inside the container
WORKDIR /app

# Copy the source code into the container
COPY main.go .

# Build the Go application
RUN go build -o main main.go

# Run the application
CMD ["./main"]