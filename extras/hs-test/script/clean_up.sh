#!/bin/bash

# Stop all running containers
containers=$(docker container ls -aq)
if [ -z "$containers" ]; then
    echo "No containers are running."
else
    docker container stop $containers
    echo "All running containers have been stopped."
fi

# Remove all containers
if [ -z "$containers" ]; then
    echo "No containers to remove."
else
    docker container rm $containers
    echo "All containers have been removed."
fi

# Remove all Docker images
images=$(docker images -q)
if [ -z "$images" ]; then
    echo "No Docker images to remove."
else
    docker rmi $images
    echo "All Docker images have been removed."
fi
