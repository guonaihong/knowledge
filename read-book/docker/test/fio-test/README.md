以下是一个简单的 Dockerfile 脚本，用于创建一个包含 fio 命令的 Docker 镜像：

## Dockerfile

```Dockerfile
# 使用官方的 Ubuntu 镜像作为基础镜像
FROM ubuntu:latest

# 设置工作目录
WORKDIR /app

# 更新包列表并安装 fio
RUN apt-get update && \
    apt-get install -y fio && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# 默认命令
CMD ["fio", "--version"]
```

这个 Dockerfile 脚本做了以下几件事：

1. 使用官方的 Ubuntu 镜像作为基础镜像。
1. 设置工作目录为 /app。
1. 更新包列表并安装 fio，然后清理包列表以减小镜像大小。
1. 设置默认命令为 fio --version，以便在运行容器时显示 fio 的版本信息。

要构建这个镜像，请将上述内容保存到一个名为 Dockerfile 的文件中，然后在终端中运行以下命令：

```bash
docker build -t fio-image .
```

这将构建一个名为 fio-image 的 Docker 镜像。你可以使用以下命令来运行这个镜像：

```bash
docker run --rm fio-image
```

这将显示 fio 的版本信息。如果你想运行其他 fio 命令，可以在 docker run 命令中指定它们，例如：

```bash
docker run --rm fio -direct=1 -iodepth=64 -rw=read -ioengine=libaio -bs=4k -size=10G -numjobs=1  -name=./fio.test
```

这将运行 fio 并使用 /path/to/test 目录中的 fio-job-file.fio 文件作为配置文件。请确保将 /path/to/test 替换为你实际的测试目录路径。
