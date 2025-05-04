# Build image
docker build -t itenary-service .

# Run container
docker run -p 8080:8080 itenary-service