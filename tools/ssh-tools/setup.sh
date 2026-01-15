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
mkdir -p "${GENET_DIR}/lib"
mkdir -p "${GENET_DIR}/etc/ssh"

# 复制 SSH 工具和库
echo "Copying SSH tools..."
cp /tools/bin/sshd "${GENET_DIR}/bin/sshd"
cp /tools/bin/sftp-server "${GENET_DIR}/bin/sftp-server"
cp /tools/bin/ssh-keygen "${GENET_DIR}/bin/ssh-keygen"

# 复制依赖库
echo "Copying libraries..."
cp /tools/lib/* "${GENET_DIR}/lib/" 2>/dev/null || true

# 设置执行权限
chmod +x "${GENET_DIR}/bin/sshd"
chmod +x "${GENET_DIR}/bin/sftp-server"
chmod +x "${GENET_DIR}/bin/ssh-keygen"

# 创建包装脚本（自动设置 LD_LIBRARY_PATH）
cat > "${GENET_DIR}/bin/sshd-run" << 'EOF'
#!/bin/sh
GENET_DIR=/workspace/.genet
export LD_LIBRARY_PATH="${GENET_DIR}/lib:$LD_LIBRARY_PATH"
exec "${GENET_DIR}/bin/sshd" "$@"
EOF
chmod +x "${GENET_DIR}/bin/sshd-run"

# 预生成 SSH host keys
echo "Generating SSH host keys..."
export LD_LIBRARY_PATH="${GENET_DIR}/lib:$LD_LIBRARY_PATH"
"${GENET_DIR}/bin/ssh-keygen" -t rsa -b 4096 -f "${GENET_DIR}/etc/ssh/ssh_host_rsa_key" -N "" -q 2>/dev/null || true
"${GENET_DIR}/bin/ssh-keygen" -t ecdsa -b 521 -f "${GENET_DIR}/etc/ssh/ssh_host_ecdsa_key" -N "" -q 2>/dev/null || true
"${GENET_DIR}/bin/ssh-keygen" -t ed25519 -f "${GENET_DIR}/etc/ssh/ssh_host_ed25519_key" -N "" -q 2>/dev/null || true

# 创建 VS Code Server 目录（主容器会链接到这里）
echo "Creating VS Code Server directory..."
mkdir -p "/workspace/.vscode-server"

# 写入版本标记
echo "${TOOLS_VERSION}" > "${GENET_DIR}/.version"

echo "=== Setup complete ==="
echo "SSH tools installed to: ${GENET_DIR}/"
echo "  - bin/sshd"
echo "  - bin/sftp-server"
echo "  - bin/ssh-keygen"
echo "  - bin/sshd-run (wrapper with LD_LIBRARY_PATH)"
echo "  - lib/ (shared libraries)"
