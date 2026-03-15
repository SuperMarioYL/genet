# 工作负载定时挂起设计

## 背景

当前 `backend/cmd/cleanup` 的定时任务只会遍历用户 namespace 下的 Pod，并删除未受保护的 Pod。这满足单 Pod 工作区的“每日清理”需求，但不适用于 `Deployment` 和 `StatefulSet`：

- 用户希望保留当前运行态，第二天可快速恢复
- 恢复后希望继续使用挂起前保存出来的镜像
- 工作负载卡片需要体现“挂起”状态，并支持手动恢复

## 目标

- 工作负载类型为 `pod` 时，继续保持“每天定时清理”
- 工作负载类型为 `deployment` 或 `statefulset` 时，定时任务改为“挂起”
- 挂起时只选择一个代表 Pod 执行 commit，恢复时所有副本统一切换为该 commit 镜像
- 挂起后将工作负载副本数缩容为 `0`
- 前端卡片显示 `挂起`，并提供 `恢复` 操作

## 非目标

- 不为每个副本分别保存镜像
- 不引入新的数据库或外部状态存储
- 不实现用户自定义代表 Pod 选择策略

## 代表 Pod 选择

采用固定策略：

1. 优先选择名字排序后第一个 `Running` 的 Pod
2. 如果没有 `Running` Pod，则选择名字排序后第一个未终止 Pod
3. 如果没有可选 Pod，则本次挂起失败并记录原因

这样不需要新增配置，行为稳定且容易测试。

## 方案选型

### 方案 A：在现有 cleanup 流程中分流

- `pod` 保持现有删除逻辑
- `deployment` / `statefulset` 在 cleanup 中执行 commit、记录状态、缩容
- 新增恢复接口，由前端卡片触发

优点：

- 复用现有 CronJob 和 commit job 能力
- 改动面集中在已有后端路径
- 前端只需在现有卡片上补状态和动作

缺点：

- cleanup 逻辑会从“只删 Pod”演进成“删除 + 挂起”双路径

### 方案 B：新增独立 suspend controller

- 单独实现一套挂起调度器
- cleanup 仍只负责删除 Pod

优点：

- 概念上更清晰

缺点：

- 对当前项目过重
- 会重复一部分扫描和 commit 逻辑

## 选定方案

采用方案 A，在现有 cleanup 流程中按工作负载类型分流。

## 后端设计

### 状态存储

挂起状态直接存储在 `Deployment` / `StatefulSet` annotation 上，不新增独立存储。

新增注解：

- `genet.io/suspend-enabled=true`
- `genet.io/suspended=true|false`
- `genet.io/suspended-replicas=<原副本数>`
- `genet.io/suspended-image=<commit 成功后的镜像>`
- `genet.io/suspended-source-pod=<代表 pod>`
- `genet.io/suspended-at=<RFC3339 时间>`
- `genet.io/suspend-message=<最近一次状态说明>`

创建 `Deployment` / `StatefulSet` 时就写入 `genet.io/suspend-enabled=true`，便于后续判断。

### cleanup 流程

`CleanupAllPods` 拆成两类处理：

#### 单 Pod

- 保持现有逻辑
- 继续检查 `genet.io/protected-until`
- 未保护则删除 Pod，并尝试清理 `scope=pod` 的 PVC

#### Deployment / StatefulSet

对每个工作负载执行：

1. 跳过已经 `replicas=0` 且 `genet.io/suspended=true` 的资源
2. 拉取当前副本 Pod 列表并选择代表 Pod
3. 生成自动 commit 镜像名
4. 创建 commit job 并同步等待完成
5. commit 成功后：
   - 更新 workload template image 为新镜像
   - 记录 `suspended-image`、`suspended-source-pod`、`suspended-at`
   - 记录 `suspended-replicas=<当前 replicas>`
   - 将 `spec.replicas` 改为 `0`
   - 写入 `suspended=true`
6. commit 失败时：
   - 不缩容
   - 保持原镜像
   - 更新 `suspend-message`

### commit 镜像命名

自动命名规则使用工作负载级前缀，避免和用户手工保存镜像冲突。建议格式：

`<registry>/<user>/suspend-<workload-name>:<YYYYMMDD-HHmmss>`

如果 registry 未配置，则复用现有 commit 流程的错误语义并终止挂起。

### 恢复接口

新增接口：

- `POST /api/deployments/:id/resume`
- `POST /api/statefulsets/:id/resume`

恢复逻辑：

1. 校验资源存在且 `genet.io/suspended=true`
2. 读取 `suspended-image` 和 `suspended-replicas`
3. 将 workload template image 更新为 `suspended-image`
4. 将 `spec.replicas` 恢复为 `suspended-replicas`
5. 清理挂起标记，或至少将 `suspended=false`
6. 返回新的工作负载响应

### 响应模型

`DeploymentResponse` 和 `StatefulSetResponse` 增加：

- `suspended: boolean`
- `suspendedImage?: string`
- `suspendedReplicas?: number`
- `suspendedAt?: time.Time`

状态映射新增 `Suspended`：

- 当 `spec.replicas == 0` 且 `genet.io/suspended=true` 时返回 `Suspended`
- 其余状态保持现有逻辑

## 前端设计

### API 类型

在 `frontend/src/services/api.ts` 为 `ManagedDeployment`、`ManagedStatefulSet` 增加：

- `suspended?: boolean`
- `suspendedImage?: string`
- `suspendedReplicas?: number`
- `suspendedAt?: string`

同时新增：

- `resumeDeployment(id)`
- `resumeStatefulSet(id)`

### 卡片展示

`DeploymentCard` 和 `StatefulSetCard` 调整为工作负载级状态展示：

- `status === Suspended` 时显示 `挂起`
- 原 `Ready x/y` 改为：
  - 运行中：`ready/replicas Ready`
  - 挂起：`已挂起`
- 操作区增加 `恢复` 按钮
- 挂起时子 Pod 列表显示空态提示，不再依赖已有 Pod 卡片

`PodCard` 保持单 Pod 逻辑，不承担工作负载恢复动作。

## 错误处理

- 代表 Pod 不存在：不缩容，记录失败原因
- commit job 失败：不缩容，记录失败原因
- 恢复时缺少 `suspended-image` 或 `suspended-replicas`：返回 `400`
- 恢复时工作负载不存在：返回 `404`
- 恢复时未处于挂起状态：返回 `409`

## 测试策略

### 后端

- cleanup:
  - `pod` 仍然按旧逻辑删除
  - `deployment` / `statefulset` commit 成功后缩容为 `0`
  - commit 失败时不缩容
- handlers:
  - 恢复接口读取挂起注解并恢复镜像和副本数
  - 响应状态映射为 `Suspended`

### 前端

- `DeploymentCard` / `StatefulSetCard` 在挂起态显示 `挂起`
- 点击 `恢复` 会调用对应 API 并刷新
- 挂起态不展示运行中 Pod 卡片列表
