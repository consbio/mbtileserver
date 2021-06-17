# Deployment

## AWS ECS (Fargate) + ALB (Application Load Balancer)

The example handles tasks such as generating a docker image and setting up an application load balancer (ALB) and ECS services.

1. Install CDK and connect to your AWS account. This step is only necessary once per AWS account.

```bash
# Download forked mbtileserver repo
$ git clone https://github.com/NASA-IMPACT/mbtileserver.git

# install cdk dependencies
$ cd mbtileserver/deployment
$ pip install -r requirements.txt
$ npm install

$ npm run cdk bootstrap # Deploys the CDK toolkit stack into an AWS environment

# or, only deploy to a specific region
$ npm run cdk bootstrap aws://${AWS_ACCOUNT_ID}/eu-central-1
```

2. Generate CloudFormation template

This step isn't required, but can be useful to just validate that the configuration.

```bash
$ npm run cdk synth  # Synthesizes and prints the CloudFormation template for this stack
```

3. Update settings (see [intro.md](intro.md)) either as exported shell params or in .env file:

```bash
export MBTILESERVER_STACK_NAME="mbtileserver"
export MBTILESERVER_STACK_STAGE="dev"

# s3 path to mbtiles files
export MBTILESERVER_STACK_MBTILES_PATH_PREFIX="my-bucket/my-mbtiles/"

# this defines the tag of the Docker image to use
export MBTILESERVER_STACK_IMAGE_VERSION="057db5a-2021-06-17-14-47-37"

# change the ECS instance resource configuration
export MBTILESERVER_STACK_MIN_ECS_INSTANCES=10
export MBTILESERVER_STACK_MAX_ECS_INSTANCES=50
export MBTILESERVER_STACK_TASK_CPU=1024
export MBTILESERVER_STACK_TASK_MEMORY=3072 # enough to keep all mbtiles files in memory?
```

Valid CPU and Memory values:

```
# CPU value      |   Memory value
# 256 (.25 vCPU) | 0.5 GB, 1 GB, 2 GB
# 512 (.5 vCPU)  | 1 GB, 2 GB, 3 GB, 4 GB
# 1024 (1 vCPU)  | 2 GB, 3 GB, 4 GB, 5 GB, 6 GB, 7 GB, 8 GB
# 2048 (2 vCPU)  | Between 4 GB and 16 GB in 1-GB increments
# 4096 (4 vCPU)  | Between 8 GB and 30 GB in 1-GB increments
```

4. Deploy

Deployment presents a chicken-and-egg problem.  The deployment creates the ECR repository, but also needs the
images that have not yet been deployed to it to run the Tasks. We solve this by first doing a deployment that fails
(but creates the ECR repository), then publishing the Docker image to the ECR, and finally doing another deployment
that succeeds. Alternately, manually create an ECR repository called 'mbtileserver' and push the Docker image to it 
before the first deploy.

```bash
# Deploys the stack(s) mbtileserver-ecs-dev in cdk/app.py
$ npm run cdk deploy mbtileserver-ecs-dev  # or whatever MBTILESERVER_STACK_STAGE is set to
```

This will fail if the ECR repo hasn't been created manually or by a previous deployment.

5. Build mbtileserver Docker image and push to ECR

**Note**: your local Docker host must be configured with at least 4GB of memory for this to build correctly.

```bash
$ cd ../  # to mbtileserver root directory
$ make
```

6. Update the image version and redeploy

Configure `MBTILESERVER_STACK_IMAGE_VERSION` to be the image version you want to deploy, e.g., the output of your previous make.

Then, deploy again:

```bash
# Deploys the stack(s) mbtileserver-ecs-dev in cdk/app.py
$ npm run cdk deploy mbtileserver-ecs-dev  # or whatever MBTILESERVER_STACK_STAGE is set to
```

This time, it should succeed

7. Restart

The mbtiles files are copied locally when the container starts up. When the files are updated in S3, it is necessary
to restart the ECS Tasks to uptake these changes. There are several ways to do this but the easiest way is to simply 
run `cdk deploy` again.
