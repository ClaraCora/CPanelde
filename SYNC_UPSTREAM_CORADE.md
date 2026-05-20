# Corade 上游同步与品牌重套标准流程

本文档用于指导后续 AI 或人工，以**固定、可重复**的方式把上游 [`upstream/dev`](.git/config:16) 同步到当前仓库，并重新套用 Corade 的定制要求。

适用场景：
- 上游 [`cedar2025/Xboard-Node`](.git/config:17) 有新提交
- 当前仓库需要继续保留 Corade 品牌
- 需要同步上游功能，同时维持现有安装路径、文档、镜像与 CLI 命名约定

---

## 目标约束

同步后必须满足以下要求：

### 1. 品牌命名
- 主项目名使用 [`Corade`](README.md:1)
- 主二进制名使用 [`corade`](Dockerfile:24)
- 管理 CLI 使用 [`coradectl`](cmd/xbctl/main.go:29)
- systemd 服务名使用 [`corade.service`](cmd/xbctl/main.go:30)

### 2. 路径与运行目录
- 安装根目录：[`/etc/corade`](cmd/xbctl/main.go:32)
- 主配置：[`/etc/corade/config.yml`](cmd/xbctl/main.go:25)
- 凭据文件：[`/etc/corade/credentials.env`](cmd/xbctl/main.go:27)
- 元数据文件：[`/etc/corade/install-meta.json`](cmd/xbctl/main.go:26)
- 二进制路径：[`/usr/local/bin/corade`](cmd/xbctl/main.go:28)
- CLI 路径：[`/usr/local/bin/coradectl`](cmd/xbctl/main.go:29)

### 3. 远程资源
- Release 下载源：[`https://github.com/ClaraCora/coradem/releases`](cmd/xbctl/main.go:33)
- 安装脚本远程地址：[`https://raw.githubusercontent.com/ClaraCora/coradem/main/install.sh`](README.md:38)
- Docker 镜像：[`ghcr.io/claracora/corade`](.github/workflows/ci.yml:101)

### 4. 运行时外显要求
尽量避免安装后在 VPS 终端直接看到上游项目名称：
- 不显示 `xboard-node`
- 不显示 `xbctl`
- 尽量不显示上游 GitHub 链接
- 保留必要的 Corade 自有链接可接受

### 5. 新架构保留要求
如果上游新增核心能力，原则上**优先接入**，包括但不限于：
- 新安装器架构 [`install.sh`](install.sh)
- 新 CLI [`cmd/xbctl/main.go`](cmd/xbctl/main.go)
- 新配置模型 [`internal/config/config.go`](internal/config/config.go)
- multi-instance / machine 模式 [`internal/machine/machine.go`](internal/machine/machine.go)
- 新文档、扩展说明、校验逻辑、控制平面逻辑

---

## 标准同步步骤

## Step 0：确认工作区干净
先确认没有未提交改动：

```bash
git status --short
```

如果有改动，先提交或备份。

---

## Step 1：抓取上游
如果上游拉取需要 SSH 私钥，统一使用：

```bash
GIT_SSH_COMMAND='ssh -i /root/.miyao/my_ed25519_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new' git fetch upstream
```

然后查看分支关系：

```bash
git log --oneline --decorate --graph --max-count=20 --all
```

目标通常是把 [`upstream/dev`](.git/config:16) 合并到当前 [`dev`](.:1)。

---

## Step 2：执行合并

```bash
git merge upstream/dev
```

如果无冲突，直接进入后续验证。

如果有冲突，继续执行下面步骤。

---

## Step 3：列出冲突文件

```bash
git diff --name-only --diff-filter=U
```

也可以扫冲突标记：

```bash
grep -RIn '^<<<<<<<\|^=======\|^>>>>>>>' . --exclude-dir=.git
```

常见高冲突文件：
- [`install.sh`](install.sh)
- [`README.md`](README.md)
- [`README-cn.md`](README-cn.md)
- [`cmd/xboard-node/main.go`](cmd/xboard-node/main.go)
- [`cmd/xbctl/main.go`](cmd/xbctl/main.go)
- [`config.yml.example`](config.yml.example)
- [`internal/config/config.go`](internal/config/config.go)
- [`internal/cert/cert.go`](internal/cert/cert.go)
- [`Makefile`](Makefile)
- [`.github/workflows/ci.yml`](.github/workflows/ci.yml)

---

## Step 4：冲突处理总原则

### 总原则
**优先保留上游结构，随后重新套用 Corade 品牌与运行约束。**

不要反过来做。原因：
- 上游功能更新通常是结构级变化
- 先保留本地旧文件，容易把新架构丢掉
- 正确策略是：**先吃掉上游，再重命名/重路径/重文案**

### 具体策略
对冲突文件，优先做：

```bash
git checkout --theirs <file>
```

即先采用上游版本，然后再进行 Corade 重套。

适合直接保留上游结构的文件：
- [`install.sh`](install.sh)
- [`cmd/xboard-node/main.go`](cmd/xboard-node/main.go)
- [`cmd/xbctl/main.go`](cmd/xbctl/main.go)
- [`internal/config/config.go`](internal/config/config.go)
- [`internal/config/config_test.go`](internal/config/config_test.go)
- [`internal/cert/cert.go`](internal/cert/cert.go)
- [`config.yml.example`](config.yml.example)
- [`Makefile`](Makefile)
- [`.github/workflows/ci.yml`](.github/workflows/ci.yml)

文档类文件同样建议先接上游，再重写品牌：
- [`README.md`](README.md)
- [`README-cn.md`](README-cn.md)

---

## Step 5：Corade 重套规则

### 5.1 基础品牌替换
需要统一替换的核心映射：

| 上游 | 当前要求 |
| --- | --- |
| `xboard-node` | `corade` |
| `xbctl` | `coradectl` |
| `/etc/xboard-node` | `/etc/corade` |
| `/usr/local/bin/xboard-node` | `/usr/local/bin/corade` |
| `/usr/local/bin/xbctl` | `/usr/local/bin/coradectl` |
| `xboard-node.service` | `corade.service` |
| `ghcr.io/cedar2025/xboard-node` | `ghcr.io/claracora/corade` |
| `https://github.com/cedar2025/xboard-node/releases` | `https://github.com/ClaraCora/coradem/releases` |
| `https://raw.githubusercontent.com/cedar2025/xboard-node/dev/install.sh` | `https://raw.githubusercontent.com/ClaraCora/coradem/main/install.sh` |

### 5.2 需要重点人工复核的文件
#### [`install.sh`](install.sh)
必须检查：
- `APP_NAME`
- `INSTALL_ROOT`
- `BINARY_PATH`
- `CLI_PATH`
- `SERVICE_NAME`
- `DEFAULT_DOWNLOAD_BASE`
- service 模板中的 `Description`
- 任何 `Documentation=` 字段
- 所有 `xboard-node` / `xbctl` 二进制下载、备份、临时文件、软链接
- `/usr/bin/xbctl` 是否已改成 `/usr/bin/coradectl`

#### [`cmd/xbctl/main.go`](cmd/xbctl/main.go)
必须检查：
- 版本输出是否改成 [`coradectl`](README.md:45)
- usage 文案是否还有 `xbctl`
- 下载文件名、临时文件名、回滚文件名是否已改
- service 文件重生成逻辑是否仍写旧服务名
- 默认路径常量是否都已改成 Corade 路径

#### [`README.md`](README.md)
必须检查：
- 标题是否为 [`# Corade`](README.md:1)
- 安装命令是否为 Corade 仓库地址
- Docker 镜像是否为 `ghcr.io/claracora/corade`
- CLI 章节是否为 [`coradectl`](README.md:45)
- 不再保留上游项目链接与命令名

#### [`README-cn.md`](README-cn.md)
中文文档原则同英文文档一致。

#### [`Makefile`](Makefile)
必须检查：
- 产物名：`corade` / `coradectl`
- Linux 构建产物：`corade-linux-*` / `coradectl-linux-*`
- clean/install/docker 目标中的名称是否一致

#### [`.github/workflows/ci.yml`](.github/workflows/ci.yml)
必须检查：
- artifact 名称
- release 上传文件名
- Docker 推送镜像名

---

## Step 6：推荐检查命令

### 检查是否还有冲突未解决

```bash
git diff --name-only --diff-filter=U
```

为空才算解决完成。

### 检查是否仍残留上游公开标识

```bash
grep -RIn 'xboard-node\|xbctl\|cedar2025/xboard-node' \
  install.sh README.md README-cn.md config.yml.example \
  Makefile Dockerfile .github/workflows/ci.yml \
  cmd/xbctl/main.go cmd/xboard-node/main.go \
  internal/config/config.go internal/config/config_test.go internal/cert/cert.go \
  docs-custom-routes.md docs-custom-outbounds.md || true
```

注意：
- import 路径里出现 [`github.com/cedar2025/xboard-node`](cmd/xboard-node/main.go:17) 暂时允许
- 这是源码级标识，不是运行时外显问题
- 不要在这一步强行改 `module` 和 import 路径，除非准备整体重命名 Go module

### 检查工作区状态

```bash
git status --short
```

---

## Step 7：验证

统一至少执行：

```bash
go test ./...
```

如需进一步验证构建：

```bash
make build
```

如果新安装器/新 CLI 有大改动，建议额外验证：
- [`install.sh`](install.sh) 参数帮助
- [`coradectl`](README.md:45) 基本命令输出
- systemd service 文件内容
- 默认路径与文件生成逻辑

---

## Step 8：提交与推送

```bash
git add -A
git commit -m "merge: sync upstream dev and reapply corade branding"
GIT_SSH_COMMAND='ssh -i /root/.miyao/my_ed25519_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new' git push origin HEAD
```

---

## 建议的 AI 操作模板

后续 AI 可以按这个固定顺序执行：

1. 查看状态：
```bash
git status --short && git remote -v
```

2. 拉上游：
```bash
GIT_SSH_COMMAND='ssh -i /root/.miyao/my_ed25519_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new' git fetch upstream
```

3. 合并：
```bash
git merge upstream/dev
```

4. 列冲突：
```bash
git diff --name-only --diff-filter=U
```

5. 对核心文件优先采用上游结构：
```bash
git checkout --theirs .github/workflows/ci.yml Makefile README.md cmd/xboard-node/main.go config.yml.example install.sh internal/cert/cert.go internal/config/config.go internal/config/config_test.go
```

6. 然后重套 Corade 规则：
- `xboard-node -> corade`
- `xbctl -> coradectl`
- `/etc/xboard-node -> /etc/corade`
- `/usr/local/bin/xboard-node -> /usr/local/bin/corade`
- `/usr/local/bin/xbctl -> /usr/local/bin/coradectl`
- `xboard-node.service -> corade.service`
- `ghcr.io/cedar2025/xboard-node -> ghcr.io/claracora/corade`
- `https://github.com/cedar2025/xboard-node/releases -> https://github.com/ClaraCora/coradem/releases`

7. 再检查残留：
```bash
grep -RIn 'xboard-node\|xbctl\|cedar2025/xboard-node' install.sh README.md README-cn.md config.yml.example Makefile Dockerfile .github/workflows/ci.yml cmd/xbctl/main.go cmd/xboard-node/main.go internal/config/config.go internal/config/config_test.go internal/cert/cert.go docs-custom-routes.md docs-custom-outbounds.md || true
```

8. 测试：
```bash
go test ./...
```

9. 提交推送：
```bash
git add -A && git commit -m "merge: sync upstream dev and reapply corade branding"
GIT_SSH_COMMAND='ssh -i /root/.miyao/my_ed25519_key -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new' git push origin HEAD
```

---

## 注意事项

### 不要做的事
- 不要在同步上游时优先保留旧版 [`install.sh`](install.sh)
- 不要在未确认测试通过前直接推送
- 不要把源码 import 路径中的 [`github.com/cedar2025/xboard-node`](cmd/xboard-node/main.go:17) 当作第一优先级处理
- 不要把“运行时外显清理”和“Go module 改名”混为一谈

### 建议优先级
1. 上游结构先接住
2. CLI 和安装器重命名
3. 文档/工作流/镜像改名
4. 运行时外显清理
5. 测试通过
6. 再推送

---

## 本仓库当前已验证过的一次成功流程结果
参考已完成的同步提交：
- [`merge: sync upstream dev and reapply corade branding`](.:1)

这说明上述流程在当前仓库上已经验证过，可以作为后续 AI 的标准操作模板。