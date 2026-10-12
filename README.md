# client-guard：CPA 按 API Key 限制客户端插件（v0.1.2）

CLIProxyAPI（CPA）v8 原生动态库插件（C ABI，`-buildmode=c-shared`）。按**下游 API Key** 记录并限制可使用的客户端软件（依据客户端发来的原始请求头：`User-Agent`、`Originator`、`X-Title`、`HTTP-Referer`、`Referer`、`X-Client-Name` 以及可配置的自定义头，默认 `X-Client`），并在 CPA 管理端提供中文管理页面。

> ⚠️ 测试版，未发布到插件商店。User-Agent 可以伪造，这是**软限制**：能防止把 Key 随手接到 Cherry Studio 等工具上，防不住刻意伪装。

## 工作方式

- 挂在 CPA 的 `RequestInterceptor.InterceptRequestBeforeAuth`（`request.intercept_before`）阶段：在选择上游账号、调用上游**之前**执行。
- 插件看到的是**客户端原始请求头**（CPA 从 gin 请求复制而来），Codex 伪装（cloaking）在之后的 executor 阶段才改写发往上游的 UA，互不影响。已用本地 CPA v8.0.3 实测：插件记录 `CherryStudio/1.5.3`，而上游收到的 UA 是 CPA 改写后的值。
- Key 身份来自 CPA 提供的 `Metadata.caller_scope`（Key 的加盐 SHA-256）。插件不接触明文 Key；页面通过管理接口 `/v0/management/api-keys` 在浏览器端计算哈希，显示 Key 末 6 位。
- 拦截时返回 `403`：`{"error":{"type":"client_not_allowed","code":"client_not_allowed","message":"…","user_agent":"…"}}`，请求不会发到上游。

## 规则

| 模式 | 说明 |
|---|---|
| `off` | 不记录、不拦截 |
| `log-only` | 只记录（默认）。装上不会挡任何人 |
| `enforce` | 不在允许列表内 → 403；允许列表为空时视为不限制 |
| `inherit` | 仅单个 Key 可用：沿用全局默认 |

- 允许列表每项三种写法，**全部不区分大小写**（头名和值都是）：
  - 普通文本：子串匹配，对所有识别的请求头逐个匹配，任一命中即放行。例：`codex`。
  - `re:正则`：插件自动加 `(?i)`，对每个请求头的值分别匹配，`^` 锚定的是**每个头值的开头**。例：`re:^codex[_-]`。写 `re:(?i)...` 也可以，效果相同。
  - `header:请求头=值`：只匹配指定的那个头，值同样支持普通文本或 `re:`。例：`header:X-Title=Cherry Studio`、`header:X-Client=re:^claude-desktop$`。`HTTP-Referer` 和 `Referer` 是两个不同的头。
- 识别的请求头：`User-Agent`、`Originator`、`X-Title`、`HTTP-Referer`、`Referer`、`X-Client-Name`，加上 `extra_headers`（默认 `[X-Client]`）和规则里 `header:` 引用到的头。`Authorization`、`X-Api-Key`、`Cookie` 等敏感头永远不读、不记，也不能写进规则。
- 0.1.2 起普通文本规则也会匹配 X-Title / Referer 等新增头，所以旧的 `codex` 规则会多放行“X-Title 或 Referer 里带 codex”的请求；Codex 官方客户端不受影响。页面会提示把 `codex` 换成 `re:^codex[_-]`，但**不会自动修改**。
- 单个 Key 设置了模式但允许列表为空时，沿用全局允许列表。
- 客户端名称：先用最匹配的预设（具体预设优先，例如「Codex 桌面版」优先于「Codex 全家」），否则取 UA 的产品名。
- 「一键加入允许列表」生成的精确特征：命中具体预设时用预设特征；否则依次用 `header:Originator=re:^值$`、自定义头、`X-Client-Name`、`X-Title`、UA 产品名 `header:User-Agent=re:^产品名/`。

### 内置预设

| 分类 | 预设 | 特征 | 置信度 |
|---|---|---|---|
| Codex | Codex 全家 | `re:^codex[_-]` | 源码确认 |
| Codex | Codex CLI | `re:^(codex_cli_rs\|codex-tui\|codex_exec\|codex-cli)([/ ]\|$)` | 源码确认 |
| Codex | Codex 桌面版 | `re:^codex_desktop([/ ]\|$)` | 源码确认（桌面端未实抓） |
| Codex | Codex IDE 插件 | `re:^codex_vscode([/ ]\|$)` | 源码确认 |
| 编码 CLI | Claude Code | `re:^claude-cli/` | 文档/抓包 |
| 编码 CLI | Gemini CLI | `re:^(GeminiCLI\|CloudCodeVSCode)` | 源码确认 |
| 编码 CLI | Qwen Code | `re:^QwenCode/` | 源码确认 |
| 编码 CLI | OpenCode | `re:^opencode/` | 源码确认 |
| 编码 CLI | DeepSeek Harness | `re:^deepseek-harness/` | 源码确认 |
| 编码 CLI | Continue CLI | `re:^Continue-CLI/` | 源码确认 |
| IDE 插件 | Cline | `re:^Cline/` | 源码确认 |
| IDE 插件 | Roo Code | `re:^roo-code([/ ]\|$)`（仅 OpenAI 原生 provider） | 源码确认 |
| IDE 插件 | Kilo Code | `re:^kilo/` | 源码确认 |
| 桌面客户端 | Cherry Studio | `header:X-Title=re:^Cherry Studio$`、`header:HTTP-Referer=cherry-ai.com`、`header:Originator=re:^cherry-studio$` | 源码确认 |
| 桌面客户端 | Claude Desktop | `header:X-Client=re:^claude-desktop$`（需在 Claude Desktop 配置 `inferenceCustomHeaders: {"X-Client": "claude-desktop"}`） | 文档 |
| 桌面客户端 | WorkBuddy / CodeBuddy | 无（待日志确认） | 推测 |
| 调试工具 | curl / HTTPie / Postman / Apifox | `re:^curl/`、`re:^HTTPie/`、`re:^PostmanRuntime/`、`re:^Apifox/` | 文档 |
| SDK | OpenAI SDK（⚠ 不建议用于放行） | `re:^OpenAI/(Python\|JS)` | 文档 |

Trae IDE 的自定义模型经 Trae 服务器转发、Cursor 类经自家云端，不做预设。详见 `客户端识别清单.md`。

## 配置（config.yaml）

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    client-guard:
      enabled: true
      priority: 1000          # 越大越先执行；建议高于 cpa-key-billing，被拦的请求不再进入计费插件
      default_mode: log-only  # off / log-only / enforce
      default_allow: ["re:^codex[_-]"]   # 0.1.2 起的默认值；已有 rules.json 时以 rules.json 为准
      extra_headers: [X-Client]   # 额外读取/记录的自定义头（0.1.2 新增）
      fail_open: true         # 插件内部出错时放行
      data_dir: plugins/client-guard   # 相对 CPA 工作目录
      recent_limit: 300
      # block_message: "自定义提示"
```

页面保存的规则写入 `<data_dir>/rules.json`，**优先于** config.yaml 里的 `default_mode/default_allow`（删掉 rules.json 即回到 config.yaml 默认）。统计写入 `<data_dir>/stats.json`（每 30 秒及退出时落盘；最多 200 个 Key × 50 种客户端，最近记录默认 300 条）。时间均为北京时间。

## Key 名称（0.1.1 起自动读取）

页面在浏览器里按以下优先级显示每个 Key 的名称，并标出来源：

| 优先级 | 来源 | 接口 | 关联方式 |
|---|---|---|---|
| 1 | 手动备注 | 本插件 rules.json 的 `label` | caller_scope |
| 2 | CPAMP 别名 | `GET /v0/management/api-key-aliases`（CPA-Manager-Plus 服务端 SQLite，仅在 CPAMP 中打开页面时可用） | `apiKeyHash = SHA-256(明文 Key)` |
| 3 | key-billing 备注 | `GET /v0/management/plugins/cpa-key-billing/keys` 的 `label` | `scope` 与 caller_scope 相同 |
| — | 兜底 | `Key …末6位` | — |

CPA v8.0.3 核心的 `access.api-keys` 是纯字符串列表，没有名称字段。明文 Key 只在浏览器内存中用于计算哈希，不显示、不发给插件；后端不新增读取其他来源的逻辑。任何来源不可用（404 等）时静默跳过，在卡片标题旁显示状态。

## 页面

顶部统计卡片（`state.summary`：今日/累计请求、拦截、Key 数、实际强制的 Key 数；按天计数保存在 stats.json 的 `daily` 字段，保留 31 天，旧版 stats.json 兼容）、全局规则（分段模式切换 + 允许列表标签 + 预设 + 高级文本编辑；切到强制会弹窗确认）、Key 卡片（名称来源、客户端徽标与拦截数、生效规则说明、单独规则）、最近请求（按 Key/结果筛选）。支持浅色/深色：跟随 CPA-Manager-Plus 注入的 `theme-dark/theme-light`，否则跟随系统。截图见 `screenshots/`。

## 管理页面与接口

- 页面：`http://<CPA>:8317/v0/resource/plugins/client-guard/settings`（CPA-Manager-Plus 插件入口菜单名“客户端限制”）。跨端口打开时页面会请你输入 Management Key（只存在本标签页 sessionStorage）。
- `GET  /v0/management/plugins/client-guard/state`
- `PUT  /v0/management/plugins/client-guard/rules`
- `POST /v0/management/plugins/client-guard/clear-stats`

## 构建与测试

```bash
go vet ./... && go test -race ./...
./build.sh        # 输出 dist/linux/amd64/client-guard.so
```

需 Go ≥1.24、gcc、`CGO_ENABLED=1`。官方镜像基于 debian:bookworm（glibc 2.36）；当前产物最高依赖 `GLIBC_2.34`，可在其中加载。CentOS 7 宿主机的 glibc 无关，插件运行在容器内。

## 下载

- 发布页：https://github.com/Samuel-wyj/cpa-client-guard/releases
- 插件商店安装包：`client-guard_<版本>_linux_amd64.zip`（根目录为 `client-guard.so`），校验见 `checksums.txt`
- 手动安装包：`cpa-client-guard-<版本>.tar.gz`，安装/升级步骤见 [测试验证步骤.md](测试验证步骤.md)
