# Mdbrain

[![License](https://img.shields.io/github/license/blackstorm/mdbrain)](https://github.com/blackstorm/mdbrain/blob/main/LICENSE) [![Release](https://img.shields.io/github/v/release/blackstorm/mdbrain)](https://github.com/blackstorm/mdbrain/releases) [![GitHub Stars](https://img.shields.io/github/stars/blackstorm/mdbrain)](https://github.com/blackstorm/mdbrain)

[English](README.md) | [简体中文](README.zh-cn.md)

**Mdbrain 是一套将 [Obsidian](https://obsidian.md/) 笔记发布为可自托管网站的完整解决方案。**

支持多 Vault 发布、自动增量发布、链接解析、反向链接展示等功能，旨在为数字花园、博客、文档和教程站点提供无缝发布体验。

采用 `Bun`、`bun:sqlite`、`TSX` 与 `HTMX` 构建，架构简洁、高效且易于维护。

## 为什么选择 Mdbrain

- **真正自托管** — 不依赖 SaaS 或第三方平台，数据完全由你掌控
- **Obsidian 原生支持** — 完整支持内部链接、反向链接和 Wiki 风格引用
- **开发者友好** — 灵活集成本地存储或 S3 兼容存储后端
- **一键发布** — 已有笔记库？点击发布即可秒级上线

## 功能

- 完全自托管，部署与数据完全可控
- 支持多 Vault 独立发布
- 提供增量与全量两种发布模式
- 原生支持 Obsidian 笔记及相关资源文件
- 自动解析内部链接与反向链接
- 内置自定义域名支持，自动配置 HTTPS
- 兼容本地存储及 S3 兼容对象存储
- 可自定义站点 Logo 与 HTML 模板

## 快速开始

**一条命令快速体验：**

```bash
docker run -d \
  --name mdbrain \
  --restart unless-stopped \
  -p 8080:8080 \
  -p 9090:9090 \
  -v mdbrain:/app/data \
  -e STORAGE_TYPE=local \
  ghcr.io/blackstorm/mdbrain:latest
```

- 公开站点：`http://<你的服务器>:8080`
- 管理后台 + Publish API：`http://<你的服务器>:9090/console`（如需限制访问，请使用防火墙/ACL 或私有网络）。

安全提示：Docker 镜像默认以 `ENVIRONMENT=production` 运行，Console 会话使用 `Secure` Cookie，因此通过纯 HTTP 访问 Console 可能不可靠。建议为 Console 提供 HTTPS 访问方式（例如反向代理或私有网络），并在公开 `9090` 端口时做好访问控制。

**生产环境部署（Caddy + 自动 TLS）：**

```bash
# 1. 克隆并配置
git clone https://github.com/blackstorm/mdbrain.git
cd mdbrain
cp selfhosted/.env.example selfhosted/.env

# 2. 启动服务
docker compose --env-file selfhosted/.env \
  -f selfhosted/compose/docker-compose.caddy.yml up -d

# 3. 访问 Console
# 直接访问：http://<你的服务器>:9090/console
# 如需限制访问，可通过 HTTPS 反代或私有网络实现。
```

然后在 `/console/init` 创建管理员账号，设置 Vault，安装 Obsidian 插件即可。

完整部署指南：[selfhosted/README.zh-cn.md](selfhosted/README.zh-cn.md)

## 配置

Mdbrain 通过环境变量读取配置。

| 变量名 | 说明 | 默认值 | 必填 |
|---|---|---|---|
| `STORAGE_TYPE` | 存储后端：`local` 或 `s3` | `local` | 否 |
| `DATA_PATH` | 数据目录 | `/app/data` | 否 |
| `CADDY_ON_DEMAND_TLS_ENABLED` | 启用自动 HTTPS 证书 | `false` | 否 |
| `S3_ENDPOINT` | S3 端点地址 | — | 是（S3） |
| `S3_ACCESS_KEY` | S3 访问密钥 | — | 是（S3） |
| `S3_SECRET_KEY` | S3 私有密钥 | — | 是（S3） |
| `S3_BUCKET` | S3 存储桶名称 | `mdbrain` | 否 |
| `S3_PUBLIC_URL` | 浏览器加载资源的公开 URL | — | 是（S3） |

完整参考：[selfhosted/README.zh-cn.md](selfhosted/README.zh-cn.md#toc-environment-variables)

## 常见问题

**支持哪些存储后端？**

本地文件系统存储，以及任何 S3 兼容的对象存储（AWS S3、MinIO、RustFS、Cloudflare R2 等）。
如果使用你自己的 S3 服务，请先创建 Bucket。仓库内置的 RustFS compose 会自动完成 Bucket 初始化。

**是否支持反向链接？**

支持。Mdbrain 自动解析 Obsidian 内部链接（`[[笔记]]`），并在每个页面展示反向链接。

**图片和附件如何处理？**

笔记中引用的所有资源文件会随内容一起上传，并从同一域名或 S3 存储提供服务。

**每个 Vault 都可以使用自定义域名吗？**

可以。每个 Vault 都可以配置独立域名，通过 Caddy 的按需 TLS 自动获取 HTTPS 证书。

## 开发

依赖：Bun 1.3+、Make。

```bash
make install
make dev
```

- 公开站点：`http://localhost:8080`
- Console：`http://localhost:9090/console`

## 发布产物

### Docker 镜像

- 镜像：`ghcr.io/blackstorm/mdbrain`
- 标签：`latest`、`X.Y.Z`、`edge`（main 分支）

### Obsidian 插件

从 [GitHub Releases](https://github.com/blackstorm/mdbrain/releases) 下载 `mdbrain-plugin.zip`，解压到 `.obsidian/plugins/mdbrain/`。

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

- 服务端（`server/`）：AGPL-3.0-or-later
- Obsidian 插件（`obsidian-plugin/`）：MIT
- 部署配置（`selfhosted/`）：MIT

第三方许可证详见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
