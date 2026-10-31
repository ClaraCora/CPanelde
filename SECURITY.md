# 安全构建、安装与迁移

## 发布信任

发行使用 Ed25519 对原始 `release-manifest.sha256` 签名，签名文件为 64 字节
`release-manifest.sha256.sig`。清单包含组件版本、源提交元数据、安装脚本及二进制摘要。
安装器内置的公钥是信任锚；同渠道下载的 `.sha256` 文件本身不是发布者身份证明。
所有发行下载仅允许 HTTPS，重定向也不能降级，超时及大小均受限。

初次安装必须使用已审核的本地源码脚本，或先用独立取得并核对的公钥验证签名安装脚本。
不要把网络下载的脚本直接管道交给 root。公钥文件不能仅因为与下载包来自同一渠道就被信任。
可用 `openssl pkey -pubin -in release-public-key.pem -outform DER | openssl dgst -sha256`
与维护者通过独立渠道提供的指纹核对。Corade 源码中的公钥位于
`internal/releaseverify/release-public-key.pem`，其他两个仓库位于根目录。

例如，已将受信公钥保存为 `/etc/cpanel-release-public-key.pem` 后，在普通用户的私有临时目录中验证
CPanel 安装器。把 `vX.Y.Z` 替换为真实已发布的不可变版本；没有签名的旧发行会被拒绝：

```bash
set -eu
VERSION=vX.Y.Z
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
base="https://github.com/ClaraCora/CPP/releases/download/cpanel-$VERSION"
for asset in install.sh cpanel-version.txt release-manifest.sha256 release-manifest.sha256.sig; do
  curl --proto '=https' --proto-redir '=https' -fsSL --connect-timeout 10 \
    --max-time 120 --max-filesize 65536 "$base/$asset" -o "$asset"
done
openssl pkeyutl -verify -pubin -inkey /etc/cpanel-release-public-key.pem \
  -rawin -in release-manifest.sha256 -sigfile release-manifest.sha256.sig
awk 'NF == 2 && ($2 == "install.sh" || $2 == "cpanel-version.txt") { print; n++ }
     END { if (n != 2) exit 1 }' release-manifest.sha256 >selected.sha256
sha256sum -c selected.sha256
signed_version=$(tr -d '\r\n' <cpanel-version.txt)
test "${signed_version%%+*}" = "$VERSION"
sudo bash ./install.sh --version "$VERSION"
```

Agent 优先使用已认证面板生成的命令，它同时绑定内嵌安装器摘要和面板身份公钥。
已安装且受信的新版 `coradectl upgrade --version vX.Y.Z` 会自行验证发布签名。
`--binary PATH` 仅是操作员明确指定的本地文件覆盖模式，文件信任由操作员负责。
签名验证失败不会回退到未签名下载，也不会先停止运行中的服务。

## 首次启用签名发布

1. 两个源码仓库的 Actions 都需要 `CPP_RELEASE_TOKEN` 和 `RELEASE_SIGNING_PRIVATE_KEY`。
   前者只授予公开 CPP 仓库 Contents 读写权限；后者填入匹配仓库公钥的 Ed25519 PEM 私钥。
   通过 GitHub Secrets 安全界面导入，避免在终端日志、聊天、提交或工单中粘贴私钥。
2. 本次生成的私钥保存在本机 `%LOCALAPPDATA%\CPanel-release-signing\release-ed25519-private.pem`，
   所在目录受 ACL 限制，未纳入任何仓库。应做受控的加密备份；丢失后不能用任意新密钥签发给旧信任锚。
   只有发布者需要这份文件；普通用户、面板和 Agent 只需要公钥。
3. 推送前审查完整改动，尤其确保 `third_party` 本地补丁已包含在 Corade 提交中。
   安全门禁通过后先发布新的 Corade 不可变版本（本次源码 VERSION 为 v2.0.8），
   再发布包含新安装器和数据库迁移的 CPanel；旧版不可变 tag 不覆盖。
4. 旧 Agent/CLI 没有新的验签入口，第一次迁移须由操作员使用已审核/验签的新安装器完成，
   后续才能依赖新的自动升级链。不要让旧的 mutable-main 自动升级路径承担首次信任建立。
5. 本地验证不能替代发布 CI。GitHub CI 的 Linux race、Alpine/OpenRC/systemd 回归、
   两架构漏洞扫描及实际产物验签必须在发布前通过；部署记录应明确对应的源提交和发行版本。

工作流在缺失或不匹配的签名私钥时直接失败。公钥签名可阻止仅有发布存储写权限的攻击者替换资产；
若签名私钥或有权读取该密钥的 CI 同时失陷，需执行独立的密钥轮换和受信客户端迁移。

## Agent V2 迁移

- 新库默认关闭旧协议；保留有活动旧机器的现有部署的显式兼容开关。迁移完成后关闭旧协议。
- 任何已经登记过 V2 身份或已标记 V2 的机器，都不能回退到共享密钥或旧机器令牌认证。
- 后台生成安装命令改为受 CSRF 保护的 POST。命令使用机器绑定、30 分钟有效、一次性登记凭据；
  重新生成会撤销上一条。数据库只短暂存放加密凭据及其哈希，登记事务消费凭据并撤销旧令牌。
- 成功登记后，Agent 持久化私有身份并删除配置、同目录 `agent.env` 及进程内的登记凭据。
  多实例配置保留实例 ID，避免删除凭据后改变身份文件目录。外部自行管理的 Secret/环境注入由操作员清理。
- 重置身份后必须重新生成并执行安装命令；单纯重启无法恢复已消费的登记凭据。
- 控制地址使用 HTTPS/WSS；仅数字回环地址允许 HTTP/WS。兼容模式也不能用公网明文或凭据重定向。

## 持续检查与例外

Go 固定最低工具链 1.27.3；前端 Node 固定 24.15.0。两架构源码扫描与实际发行二进制扫描
均是发布前门禁，Git 历史和工作树均做脱敏的 Gitleaks 检查。Actions 固定完整提交 SHA。
保留发行二进制的 Go 符号信息：当前 govulncheck 对剥离符号产物可能降为模块近似判断，
不能把该近似结果误称为实际链接了某个包。

截至 2026-10-31，源码扫描唯一剩余记录是 GO-2026-5932（未维护的 x/crypto/openpgp，未发布修复版本）。
本项目两架构的构建依赖不导入该包，六个保留符号的实际产物扫描无漏洞命中。
不对该编号做全局静默忽略；如果未来引入 OpenPGP，源码或二进制门禁必须失败并重新评估。
本地 Xray/sing-box replace 的上游身份不能仅靠版本号数据库判断，应同时审查补丁和相关回归。
