# chaitin-cli

[![CI](https://img.shields.io/github/actions/workflow/status/chaitin/chaitin-cli/ci.yml?branch=main&label=CI)](https://github.com/chaitin/chaitin-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/chaitin/chaitin-cli?label=Release)](https://github.com/chaitin/chaitin-cli/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/chaitin/chaitin-cli?label=Go)](https://github.com/chaitin/chaitin-cli/blob/main/go.mod)
[![License](https://img.shields.io/github/license/chaitin/chaitin-cli?label=License)](https://github.com/chaitin/chaitin-cli/blob/main/LICENSE)

长亭安全产品统一命令行工具

[English README](./README.en.md)

## 项目简介

`chaitin-cli` 是面向长亭安全产品的统一命令行工具，目标是在一个二进制中提供多产品的常用运维、查询和自动化能力。它解决了不同产品 API、认证方式和输出格式分散的问题，让开发者、运维人员和 AI Agent 可以用一致的方式管理 SafeLine、X-Ray、CloudWalker、T-Answer、DDR 等产品。

核心能力：

- 统一入口：通过 `chaitin-cli <product> <command>` 调用不同产品能力
- 配置复用：支持配置文件、环境变量、`.env` 和命令行参数
- 自动化友好：提供 dry-run、JSON 输出和 AI Skill，便于脚本和 Agent 集成

## 快速开始

macOS / Linux 可直接运行安装脚本：

```bash
curl -fsSL https://raw.githubusercontent.com/chaitin/chaitin-cli/main/skills/chaitin-cli/scripts/install-chaitin-cli.sh | bash
```

Windows 用户请从 [GitHub Releases](https://github.com/chaitin/chaitin-cli/releases) 下载对应版本，解压 `chaitin-cli.exe` 并加入 PATH。

## Skill

本项目提供了 skill，安装后 AI Agent（Claude Code、Cursor 等）可以直接调用 `chaitin-cli` 命令管理长亭安全产品。

如果你希望 AI Agent 自动调用 `chaitin-cli`，可以安装 Skill：

```bash
npx skills add chaitin/chaitin-cli
```

安装后，向 AI Agent 描述需求即可，例如：

- "帮我查看 SafeLine 最近的攻击日志"
- "在 X-Ray 中创建一个扫描任务"
- "列出 CloudWalker 中的漏洞事件"

## 演示

### CloudWalker
[![asciicast](https://asciinema.org/a/894643.svg)](https://asciinema.org/a/894643)

### T-Answer

[![asciicast](https://asciinema.org/a/Gs8KPOIcEnRRnXWr.svg)](https://asciinema.org/a/Gs8KPOIcEnRRnXWr)

### SafeLine

[![asciicast](https://asciinema.org/a/x7I8JDzbtjsRjb5M.svg)](https://asciinema.org/a/x7I8JDzbtjsRjb5M)

### SafeLine-CE

[![asciicast](https://asciinema.org/a/dzJzibRTm8arWRmU.svg)](https://asciinema.org/a/dzJzibRTm8arWRmU)

### DDR

[![asciicast](https://asciinema.org/a/IHulIbJ5nsy924qd.svg)](https://asciinema.org/a/t0cFuOjkLkExjREx)

### X-Ray

[![asciicast](https://asciinema.org/a/923077.svg)](https://asciinema.org/a/923077)

## 功能模块

| 模块 | 说明 |
| --- | --- |
| `chaitin` | 示例和基础命令 |
| `safeline` | SafeLine WAF 站点、策略、ACL、攻击日志和系统信息管理 |
| `safeline-ce` | SafeLine CE 站点、规则、日志、证书和增强防护管理 |
| `xray` | X-Ray 扫描任务、资产、漏洞、报告和系统配置管理 |
| `cloudwalker` | CloudWalker CWPP 事件、资产、漏洞、防护策略和系统管理 |
| `tanswer` | T-Answer 防火墙、白名单和阻断规则管理 |
| `ddr` | DDR API Token 和连接配置辅助能力 |
| `apisec` | APISec API 资产、站点、应用、访问者、数据安全和风险事件管理 |

根命令负责配置加载、产品命令注册和 BusyBox 风格调用分发；各产品目录负责自己的命令、参数、配置解析和 API 调用逻辑。

## 配置

将各产品的连接信息写入 `./config.yaml`：

```yaml
cloudwalker:
  url: https://cloudwalker.example.com/rpc
  api_key: YOUR_API_KEY

tanswer:
  url: https://tanswer.example.com
  api_key: YOUR_API_KEY

# chaitin-cli ddr get-api-token --url https://ddr.example.com:8443 --jwt-token "YOUR_JWT_TOKEN" 可以直接获取 url & api_key & company_id
ddr:
  url: "https://ddr.example.com:8443/qzh/api/v1"
  api_key: "YOUR_API_KEY"
  company_id: "YOUR_COMPANY_ID"

xray:
  url: https://xray.example.com/api/v2
  api_key: YOUR_API_KEY

apisec:
  url: https://apisec.example.com
  api_token: YOUR_API_TOKEN
```
也可以把同样的配置放到环境变量或本地 `.env` 文件中。变量命名规则为 `<PRODUCT>_<FIELD>`：

```text
cloudwalker.url      -> CLOUDWALKER_URL
cloudwalker.api_key  -> CLOUDWALKER_API_KEY
tanswer.url          -> TANSWER_URL
tanswer.api_key      -> TANSWER_API_KEY
ddr.url              -> DDR_URL
ddr.api_key          -> DDR_API_KEY
ddr.company_id       -> DDR_COMPANY_ID
xray.url             -> XRAY_URL
xray.api_key         -> XRAY_API_KEY
apisec.url           -> APISEC_URL
apisec.api_token     -> APISEC_API_TOKEN
safeline-ce.url      -> SAFELINE_CE_URL
safeline-ce.api_key  -> SAFELINE_CE_API_KEY
safeline.url         -> SAFELINE_URL
safeline.api_key     -> SAFELINE_API_KEY
```

APISec 常用查询不需要手动传内部 `scope`，优先使用语义化命令：

```bash
# 查询站点资产
chaitin-cli apisec asset site list --query count=100 --query offset=0 --output json

# 查询 API 资产
chaitin-cli apisec asset api list --query count=100 --query offset=0 --output json

# 查询风险事件
chaitin-cli apisec risk event list --query count=20 --query offset=0 --output json
```

`apisec raw` 保留为高级入口，用于调用生成出的底层 API 操作；日常查询建议先运行 `chaitin-cli apisec --help` 或对应语义命令的 `--help`。

`.env` 示例：

```bash
SAFELINE_URL=https://safeline.example.com
SAFELINE_API_KEY=YOUR_API_KEY
XRAY_URL=https://xray.example.com/api/v2
XRAY_API_KEY=YOUR_API_KEY
```

优先级为 `flags > environment/.env > config.yaml`

可以通过根命令的 `-c` 或 `--config` 指定其他配置文件。这在切换多个产品实例时很有用，例如多个 SafeLine 环境：

```bash
chaitin-cli -c ./configs/safeline-prod.yaml safeline stats overview
chaitin-cli -c ./configs/safeline-staging.yaml safeline stats overview
```

支持 dry-run 的命令可以使用根级别的 `--dry-run`：

```bash
chaitin-cli --dry-run xray plan PostPlanFilter --filterPlan.limit=10
```

## 项目结构

```text
main.go                         # 主入口、根命令、产品注册和 BusyBox 风格调用
config/                         # 配置加载、环境变量/.env 覆盖和配置写入逻辑
products/<name>/                # 每个产品一个独立目录，包含命令和 API 客户端
products/<name>/cmd/            # 手写产品命令分组
products/<name>/client/         # 生成或封装的产品 API 客户端
skills/chaitin-cli/             # AI Agent Skill 和自动安装脚本
cmd/gen-cli/                    # CLI 生成工具
Taskfile.yml                    # 构建、运行、检查和打包任务
.github/workflows/ci.yml        # CI、跨平台打包和 GitHub Release 发布流程
```

## 添加新产品

在 `products` 目录下新增产品实现

新增产品检查清单：

- 在 `main.go` 中导入产品包
- 在 `newApp()` 中通过 `a.registerProductCommand(...)` 注册命令
- 如果 `NewCommand()` 返回 `(*cobra.Command, error)`，需要在注册前处理错误
- 如果产品依赖 `config.yaml` 或根级运行时参数，在产品包里实现 `ApplyRuntimeConfig(...)`，并从 `main.go` 的 `wrapProductCommand()` 中调用
- 产品配置应在产品包内部从 `config.Raw` 解码，不要把产品字段解析逻辑塞进根命令

## BusyBox 风格调用

同一个二进制可以通过软链接，或者直接重命名后，以子命令名直接调用：

```bash
task build
ln -s ./bin/chaitin-cli ./chaitin
./chaitin
```

等价于：

```bash
./bin/chaitin-cli chaitin
```

## 开发

仓库地址：

```bash
git clone https://github.com/chaitin/chaitin-cli.git
cd chaitin-cli
```

环境准备：

- Go 版本以 [`go.mod`](./go.mod) 为准
- 安装 [Task](https://taskfile.dev/) 后可使用 `task` 命令

本地运行：

```bash
go run . chaitin
go run . safeline --help
```

常用任务:

```bash
task build
task run:chaitin
task fmt
task lint
task test
task package GOOS=linux GOARCH=amd64
```

## 维护与反馈

- 主线分支：[`main`](https://github.com/chaitin/chaitin-cli/tree/main)
- 版本发布：通过 [`GitHub Releases`](https://github.com/chaitin/chaitin-cli/releases) 提供跨平台安装包
- 发布流程：推送 `v*` tag 后由 GitHub Actions 自动测试、打包并创建 Release
- 维护状态：当前持续维护，欢迎通过 PR 贡献产品命令、文档和 bugfix

需求建议和 Bug 反馈请通过 [GitHub Issues](https://github.com/chaitin/chaitin-cli/issues) 提交。提交前请先搜索已有 Issue，避免重复反馈。请勿在公开 Issue 中粘贴 API Key、Token、真实业务地址或其他敏感信息。

## FAQ

**安装后找不到 `chaitin-cli` 怎么办？**

确认安装目录是否在 `PATH` 中。macOS / Linux 安装脚本会优先安装到用户或系统 PATH 目录；如果当前 shell 未刷新，可以重新打开终端或使用脚本输出的完整路径运行。

**配置从哪里读取？**

优先级为 `flags > environment/.env > config.yaml`。根命令的 `-c` / `--config` 可以指定其他配置文件。

**自签名证书连接失败怎么办？**

部分产品命令提供 `--insecure` 参数，SafeLine 默认跳过 TLS 证书校验；不同产品行为可能不同，可先运行对应产品命令的 `--help` 查看支持的参数。

**如何发布新版本？**

在 `main` 上创建并推送 `v*` tag，例如 `v2605.0.0`，GitHub Actions 会自动构建跨平台包并发布到 Releases。
