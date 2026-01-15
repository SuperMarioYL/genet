#!/bin/sh
# Genet SSH Tools Setup Script
# 由 InitContainer 执行，将 SSH 工具复制到共享 PVC

set -e

GENET_DIR="/workspace/.genet"
TOOLS_VERSION=$(cat /tools/VERSION 2>/dev/null || echo "unknown")

echo "=== Genet SSH Tools Setup ==="
echo "Tools version: ${TOOLS_VERSION}"

# 检查是否需要更新
if [ -f "${GENET_DIR}/.version" ]; then
    INSTALLED_VERSION=$(cat "${GENET_DIR}/.version")
    if [ "${INSTALLED_VERSION}" = "${TOOLS_VERSION}" ]; then
        echo "SSH tools already installed (version: ${INSTALLED_VERSION}), skipping..."
        exit 0
    fi
    echo "Upgrading from ${INSTALLED_VERSION} to ${TOOLS_VERSION}..."
fi

# 创建目录结构
echo "Creating directories..."
mkdir -p "${GENET_DIR}/bin"
mkdir -p "${GENET_DIR}/etc/ssh"

# 复制 SSH 工具
echo "Copying SSH tools..."
cp /tools/sshd "${GENET_DIR}/bin/sshd"
cp /tools/sftp-server "${GENET_DIR}/bin/sftp-server"
cp /tools/ssh-keygen "${GENET_DIR}/bin/ssh-keygen"

# 设置执行权限
chmod +x "${GENET_DIR}/bin/sshd"
chmod +x "${GENET_DIR}/bin/sftp-server"
chmod +x "${GENET_DIR}/bin/ssh-keygen"

# 预生成 SSH host keys（可选，主容器也会生成）
echo "Generating SSH host keys..."
"${GENET_DIR}/bin/ssh-keygen" -t rsa -b 4096 -f "${GENET_DIR}/etc/ssh/ssh_host_rsa_key" -N "" -q 2>/dev/null || true
"${GENET_DIR}/bin/ssh-keygen" -t ecdsa -b 521 -f "${GENET_DIR}/etc/ssh/ssh_host_ecdsa_key" -N "" -q 2>/dev/null || true
"${GENET_DIR}/bin/ssh-keygen" -t ed25519 -f "${GENET_DIR}/etc/ssh/ssh_host_ed25519_key" -N "" -q 2>/dev/null || true

# 创建 VS Code Server 目录（主容器会链接到这里）
echo "Creating VS Code Server directory..."
mkdir -p "/workspace/.vscode-server"

# 写入版本标记
echo "${TOOLS_VERSION}" > "${GENET_DIR}/.version"

echo "=== Setup complete ==="
echo "SSH tools installed to: ${GENET_DIR}/bin/"
echo "  - sshd"
echo "  - sftp-server"
echo "  - ssh-keygen"

