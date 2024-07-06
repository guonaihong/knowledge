# 使用一个基础镜像
FROM alpine:latest

# 创建一个无限循环的脚本
RUN echo -e "#!/bin/sh\nwhile true; do sleep 10; done" > /infinite-loop.sh

# 赋予脚本执行权限
RUN chmod +x /infinite-loop.sh

# 启动容器时运行该脚本
CMD ["/infinite-loop.sh"]