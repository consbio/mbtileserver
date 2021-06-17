.phony: all

all: docker-build docker-push

IMG_NAME = mbtileserver
$(eval AWS_ACCOUNT_ID=$(shell aws sts get-caller-identity --query Account --output text))
$(eval GIT_REV=$(shell git rev-parse HEAD | cut -c1-7))
$(eval DT=$(shell date -u +"%Y-%m-%d-%H-%M-%S"))
REV = $(GIT_REV)-$(DT)
$(eval REG=$(AWS_ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com)
$(eval REPO=$(REG)/$(IMG_NAME))

# Login to ECR
ecr-login:
	aws ecr get-login-password --region $(AWS_REGION) | docker login --password-stdin --username AWS $(REG)

# Build and tag docker image
docker-build:
	docker build -f Dockerfile --no-cache -t $(IMG_NAME) .

# Push docker image
# Don't push latest, as this can only be pushed once in an immutable ECR repo
docker-push: ecr-login
	docker tag $(IMG_NAME):latest $(REPO):$(REV)
	docker push $(REPO):$(REV)