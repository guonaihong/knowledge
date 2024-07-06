# 创建一个包含永远不会退出的进程的 Docker 容器

## 1. 创建一个 Dockerfile

首先，创建一个名为 `Dockerfile` 的文件，内容如下：

```Dockerfile
# 使用一个基础镜像
FROM alpine:latest

# 创建一个无限循环的脚本
RUN echo "while true; do sleep 10; done" > /infinite-loop.sh

# 赋予脚本执行权限
RUN chmod +x /infinite-loop.sh

# 启动容器时运行该脚本
CMD ["/infinite-loop.sh"]
```

## 2. 构建 Docker 镜像

在包含 Dockerfile 的目录中，运行以下命令来构建 Docker 镜像：

```bash
docker build -t infinite-loop-container .
```

## 3. 运行 Docker 容器

构建完成后，可以使用以下命令来运行容器：

```bash
docker run -d --name infinite-loop-container infinite-loop-container
```

这里的 -d 参数表示以分离模式（后台模式）运行容器。--name 参数用于指定容器的名称。

## 4. 验证容器状态

你可以使用以下命令来验证容器是否正在运行：

```bash
docker ps
```

你应该会看到一个名为 infinite-loop-container 的容器正在运行，并且其状态为 Up。

## 5. 停止和删除容器

如果需要停止和删除容器，可以使用以下命令：

```bash
docker stop infinite-loop-container
docker rm infinite-loop-container
```

通过这些步骤，你已经成功创建并运行了一个包含永远不会退出的进程的 Docker 容器。
