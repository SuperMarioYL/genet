# Genet SSH Tools - 静态编译 OpenSSH
# 用于 InitContainer，为任意基础镜像提供 SSH 支持

FROM alpine:latest AS builder

# 安装编译依赖
RUN apk add --no-cache \
    build-base \
    linux-headers \
    zlib-dev \
    zlib-static \
    openssl-dev \
    openssl-libs-static \
    wget

# OpenSSH 版本
ARG OPENSSH_VERSION=9.6p1

# 下载并编译 OpenSSH（静态链接）
RUN wget -q https://cdn.openbsd.org/pub/OpenBSD/OpenSSH/portable/openssh-${OPENSSH_VERSION}.tar.gz && \
    tar -xzf openssh-${OPENSSH_VERSION}.tar.gz && \
    cd openssh-${OPENSSH_VERSION} && \
    ./configure \
        --prefix=/opt/ssh \
        --sysconfdir=/etc/ssh \
        --with-privsep-path=/var/empty \
        --with-privsep-user=nobody \
        --without-pam \
        --without-selinux \
        --without-kerberos5 \
        LDFLAGS="-static" && \
    make && \
    make install-nosysconf

# 最终镜像
FROM alpine:latest

LABEL maintainer="Genet Team"
LABEL description="SSH tools for Genet InitContainer"

# 复制静态编译的 SSH 工具
COPY --from=builder /opt/ssh/sbin/sshd /tools/sshd
COPY --from=builder /opt/ssh/libexec/sftp-server /tools/sftp-server
COPY --from=builder /opt/ssh/bin/ssh-keygen /tools/ssh-keygen

# 复制安装脚本
COPY setup.sh /tools/setup.sh
RUN chmod +x /tools/setup.sh

# 版本标记
ARG VERSION=1.0.0
RUN echo "${VERSION}" > /tools/VERSION

ENTRYPOINT ["/tools/setup.sh"]

