"""Construct App."""

import os
from os.path import dirname
from typing import Any, Dict, List, Optional, Union

from aws_cdk import aws_ec2 as ec2
from aws_cdk import aws_ecs as ecs
from aws_cdk import aws_ecs_patterns as ecs_patterns
from aws_cdk import aws_iam as iam
from aws_cdk import core
from config import StackSettings
from aws_cdk import aws_ecr

settings = StackSettings()


class mbtileserverLECSStack(core.Stack):
    """mbtileserver ECS Fargate Stack."""

    def __init__(
        self,
        scope: core.Construct,
        id: str,
        cpu: Union[int, float] = 256,
        memory: Union[int, float] = 512,
        mincount: int = 1,
        maxcount: int = 50,
        permissions: Optional[List[iam.PolicyStatement]] = None,
        env: Optional[Dict] = None,
        **kwargs: Any,
    ) -> None:
        """Define stack."""
        super().__init__(scope, id, *kwargs)

        permissions = permissions or []
        env = env or {}

        vpc = ec2.Vpc(self, f"{id}-vpc", max_azs=2)

        cluster = ecs.Cluster(self, f"{id}-cluster", vpc=vpc)

        task_env = env.copy()
        task_env.update(dict(LOG_LEVEL="error"))

        repository = aws_ecr.Repository(self,
            "mbtileserver",
            repository_name="mbtileserver",
            image_scan_on_push=True,
            image_tag_mutability=aws_ecr.TagMutability.IMMUTABLE,
            removal_policy=core.RemovalPolicy.RETAIN # ECR Repo shared among all deployments
        )

        task_definition = ecs.FargateTaskDefinition(self, f"{id}-task-definition",
                                                    cpu=cpu, memory_limit_mib=memory)
        task_definition.add_container(
            f"{id}-container",
            image=ecs.ContainerImage.from_ecr_repository(
                repository, settings.image_version),
            port_mappings=[ecs.PortMapping(container_port=8000, host_port=8000)],
            environment=task_env,
            command=[settings.mbtiles_path_prefix],
            logging=ecs.LogDrivers.aws_logs(stream_prefix=id)
        )

        fargate_service = ecs_patterns.ApplicationLoadBalancedFargateService(
            self,
            f"{id}-service",
            cluster=cluster,
            desired_count=mincount,
            public_load_balancer=True,
            listener_port=80,
            task_definition=task_definition
        )
        fargate_service.target_group.configure_health_check(path="/services")

        for perm in permissions:
            fargate_service.task_definition.task_role.add_to_policy(perm)

        scalable_target = fargate_service.service.auto_scale_task_count(
            min_capacity=mincount, max_capacity=maxcount
        )

        # https://github.com/awslabs/aws-rails-provisioner/blob/263782a4250ca1820082bfb059b163a0f2130d02/lib/aws-rails-provisioner/scaling.rb#L343-L387
        scalable_target.scale_on_request_count(
            "RequestScaling",
            requests_per_target=50,
            scale_in_cooldown=core.Duration.seconds(240),
            scale_out_cooldown=core.Duration.seconds(30),
            target_group=fargate_service.target_group,
        )

        fargate_service.service.connections.allow_from_any_ipv4(
            port_range=ec2.Port(
                protocol=ec2.Protocol.ALL,
                string_representation="All port 80",
                from_port=80,
            ),
            description="Allows traffic on port 80 from ALB",
        )


app = core.App()

perms = [
    iam.PolicyStatement(
        actions=["s3:GetObject", "s3:HeadObject"],
        resources=[f"arn:aws:s3:::{settings.mbtiles_path_prefix}*"],
    ),
    iam.PolicyStatement(
        actions=["s3:ListBucket"],
        resources=[f"arn:aws:s3:::{settings.mbtiles_path_prefix.split('/', 1)[0]}"],
        conditions={"StringEquals": { "s3:prefix": settings.mbtiles_path_prefix.split('/', 1)[1]}}
    )
]

# Tag infrastructure
for key, value in {
    "Project": settings.name,
    "Stack": settings.stage,
    "Owner": settings.owner,
    "Client": settings.client,
}.items():
    if value:
        core.Tag.add(app, key, value)

ecs_stackname = f"{settings.name}-ecs-{settings.stage}"
mbtileserverLECSStack(
    app,
    ecs_stackname,
    cpu=settings.task_cpu,
    memory=settings.task_memory,
    mincount=settings.min_ecs_instances,
    maxcount=settings.max_ecs_instances,
    permissions=perms,
    env=settings.env,
)

app.synth()
