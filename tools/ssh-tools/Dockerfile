# Genet SSH Tools
# 用于 InitContainer，为任意基础镜像提供 SSH 支持
#
# 简化方案：将整个 Alpine openssh 打包
# InitContainer 会将工具复制到共享 PVC

FROM alpine:3.19

LABEL maintainer="Genet Team"
LABEL description="SSH tools for Genet InitContainer"

# 安装 OpenSSH 和必要工具
RUN apk add --no-cache \
    openssh-server \
    openssh-sftp-server \
    openssh-keygen \
    && mkdir -p /tools

# 创建一个自包含的 SSH 工具包
# 包含二进制和所有依赖库
RUN mkdir -p /tools/bin /tools/lib && \
    # 复制二进制
    cp /usr/sbin/sshd /tools/bin/ && \
    cp /usr/lib/ssh/sftp-server /tools/bin/ && \
    cp /usr/bin/ssh-keygen /tools/bin/ && \
    # 使用 ldd 找到并复制所有依赖库
    for bin in /tools/bin/*; do \
        ldd "$bin" 2>/dev/null | grep "=>" | awk '{print $3}' | while read lib; do \
            [ -f "$lib" ] && cp -n "$lib" /tools/lib/ 2>/dev/null || true; \
        done; \
    done && \
    # 复制动态链接器
    cp /lib/ld-musl-*.so.1 /tools/lib/ 2>/dev/null || true

# 创建包装脚本（设置 LD_LIBRARY_PATH）
RUN echo '#!/bin/sh' > /tools/bin/sshd-wrapper && \
    echo 'export LD_LIBRARY_PATH=/workspace/.genet/lib:$LD_LIBRARY_PATH' >> /tools/bin/sshd-wrapper && \
    echo 'exec /workspace/.genet/bin/sshd "$@"' >> /tools/bin/sshd-wrapper && \
    chmod +x /tools/bin/sshd-wrapper

# 复制安装脚本
COPY setup.sh /tools/setup.sh
RUN chmod +x /tools/setup.sh

# 版本标记
ARG VERSION=1.0.0
RUN echo "${VERSION}" > /tools/VERSION

ENTRYPOINT ["/tools/setup.sh"]
