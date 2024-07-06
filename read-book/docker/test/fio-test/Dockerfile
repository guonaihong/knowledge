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