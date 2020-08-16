SOURCE=Dockerfile
IMAGE=irto/app:latest
CONTAINER=app-server

.PHONY: image
image: Dockerfile
	docker image build -f ${SOURCE} -t ${IMAGE} .

.PHONY: shell
shell:
	docker container run -it --rm -p 8080:8080 -v ${PWD}:/go/src/work ${IMAGE}

# clean up all stopped containers
.PHONY: clean
clean:
	docker container prune

# delete all image
.PHONY: doomsday
doomsday:
	docker image rm -f `docker image ls -q`
