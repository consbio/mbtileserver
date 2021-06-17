"""MBTILESERVER_STACK Configs."""

from typing import Dict, Optional

import pydantic


class StackSettings(pydantic.BaseSettings):
    """Application settings"""

    name: str = "mbtileserver"
    stage: str = "production"

    owner: Optional[str]
    client: Optional[str]

    env: Dict = { }

    ###########################################################################
    # AWS ECS
    # The following settings only apply to AWS ECS deployment
    min_ecs_instances: int = 2
    max_ecs_instances: int = 10

    # CPU value      |   Memory value
    # 256 (.25 vCPU) | 0.5 GB, 1 GB, 2 GB
    # 512 (.5 vCPU)  | 1 GB, 2 GB, 3 GB, 4 GB
    # 1024 (1 vCPU)  | 2 GB, 3 GB, 4 GB, 5 GB, 6 GB, 7 GB, 8 GB
    # 2048 (2 vCPU)  | Between 4 GB and 16 GB in 1-GB increments
    # 4096 (4 vCPU)  | Between 8 GB and 30 GB in 1-GB increments
    task_cpu: int = 1024
    task_memory: int = 3072 

    image_version: str = "latest"

    mbtiles_path_prefix: str

    class Config:
        """model config"""

        env_file = ".env"
        env_prefix = "MBTILESERVER_STACK_"
