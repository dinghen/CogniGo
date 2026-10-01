# CogniGo

我开发并维护 CogniGo，把它作为一个面向个人使用的 AI 对话平台和 Go AI 工程练习项目。每个注册用户拥有自己的聊天模型连接、Embedding 连接、知识文件、MCP 服务和会话选择，不设置管理员角色。

## 已实现的能力

- Go、Gin、GORM 和 MySQL 提供用户、会话、消息、模型连接、知识文件和 MCP 配置 API。
- 普通对话和 SSE 流式对话使用同一套会话接口；JWT 保护需要登录的资源。
- 用户可以在配置中心保存 OpenAI-compatible 聊天模型和 Embedding 连接。API Key 加密保存，接口只返回掩码。
- Embedding 维度通过实际接口响应探测。更换模型或 endpoint 后，原始知识文件需要重新向量化，Redis Stack 服务本身不需要更换。
- RAG 使用 Markdown 标题/段落感知切片、字符上限和重叠、Embedding、Redis Stack 向量索引、相似度阈值、来源信息和无相关内容提示。当前实现是个人知识库 MVP，不包含混合检索、重排模型或人工标注集性能基准。
- 用户可以保存 Streamable HTTP 或受部署白名单控制的 stdio MCP 配置。连接测试按官方 SDK 流程执行 `Initialize -> ListTools`，工具 Schema 保存到用户自己的目录；创建会话时只挂载选中的服务和工具，调用继续经过官方 SDK `CallTool` 和 Eino 原生 Tool Calling。
- 前端提供聊天工作区、配置中心、知识文件上传/删除、模型连接测试、Embedding 索引重建、MCP 工具发现和登录注册流程。
- RabbitMQ 用于异步消息持久化，Redis 还用于验证码和短期状态；Docker Compose 编排 MySQL、Redis Stack、RabbitMQ 与 Mailpit。

## 项目结构

```text
controller/       HTTP 入参、鉴权上下文和响应
service/          业务规则、用户隔离和外部服务编排
dao/              GORM 查询与持久化边界
model/            数据库模型和 API 投影
common/aihelper/  Eino 模型、RAG 和 MCP 运行时
common/mcp/       官方 MCP Go SDK 服务端、客户端和 Eino 适配器
common/rag/       切片、Embedding、Redis 索引和检索
vue-frontend/     Vue 3 + Element Plus 前端
```

## 本地准备

需要 Go、Node.js、npm 和 Docker Compose。复制环境模板并填写自己的模型服务凭据：

```bash
cp .env.example .env
# 编辑 .env，至少填写 OPENAI_API_KEY 和 COGNIGO_PROVIDER_ENCRYPTION_KEY
make setup
make frontend-install
make infra-up
```

`.env` 只保存在本机，不要提交。默认 Compose 端口是 MySQL `3306`、Redis `6379`、RabbitMQ `5672`，如果本机已有服务，可以在 `.env` 中设置 `COGNIGO_*_PORT` 覆盖。

## 启动服务

分别在终端运行：

```bash
make run
make run-mcp
make frontend-dev
```

默认地址：

| 服务 | 地址 |
| --- | --- |
| Web 前端 | http://127.0.0.1:8080 |
| Go API | http://127.0.0.1:9090 |
| MCP Streamable HTTP | http://127.0.0.1:8081/mcp |
| Mailpit | http://127.0.0.1:8025 |
| RabbitMQ 管理界面 | http://127.0.0.1:15672 |

`/mcp` 是 MCP 协议端点，浏览器直接 GET 返回 400 或要求会话头是正常的；使用 MCP 客户端测试，不要把它当作网页页面。

## 本地演示

1. 打开前端，进入注册页，填写邮箱和密码。
2. 打开 Mailpit，读取验证码并完成注册。注册邮件和后续账号邮件都会出现在本地 Mailpit，不需要真实邮箱。
3. 登录后进入“配置中心”，添加聊天模型或 Embedding 连接；连接测试成功后再上传知识文件。
4. 在配置中心添加 MCP 服务并执行“测试并发现工具”。HTTP 服务填写 `url` 和可选 headers；stdio 需要部署端显式设置 `COGNIGO_MCP_ALLOW_STDIO=true`，并将命令加入 `COGNIGO_MCP_STDIO_COMMANDS` 白名单。
5. 回到聊天工作区创建新会话，按需选择 MCP 服务和工具。普通会话不创建 MCP Client；选择 MCP 后才会把允许的工具交给模型。

切换 Embedding 模型时，确认新的服务提供兼容的 Embeddings API。应用会探测实际维度并生成新的 Redis 索引代际，必要时在配置中心执行索引重建；不要手动填写 Redis 向量维度。

## 关键配置

完整模板在 [`.env.example`](.env.example)。常用配置包括：

| 配置 | 用途 |
| --- | --- |
| `OPENAI_API_KEY` / `OPENAI_BASE_URL` / `OPENAI_MODEL_NAME` | 默认聊天模型连接 |
| `COGNIGO_PROVIDER_ENCRYPTION_KEY` | 32 字节密钥，用于加密用户 Provider 和 MCP secrets |
| `COGNIGO_RAG_BASE_URL` / `COGNIGO_EMBEDDING_MODEL` | 默认 RAG Embedding 服务 |
| `COGNIGO_RAG_CHUNK_SIZE` / `COGNIGO_RAG_CHUNK_OVERLAP` | RAG 字符切片上限和重叠，默认 1000/150 |
| `COGNIGO_RAG_TOP_K` / `COGNIGO_RAG_DISTANCE_THRESHOLD` | 召回数量和相似度过滤 |
| `COGNIGO_MCP_ALLOW_PRIVATE_HTTP` | 是否允许本地/私网 MCP HTTP 目标，默认关闭 |
| `COGNIGO_MCP_ALLOW_STDIO` / `COGNIGO_MCP_STDIO_COMMANDS` | 是否开启 stdio 及其可执行文件白名单，默认关闭 |

用户配置优先于默认环境配置。用户输入的 API Key、HTTP headers 和 stdio env 不会通过 API 明文返回。

## 验证

```bash
make test
make vet
make build
make build-mcp
make frontend-build
```

MCP 子模块也可以单独验证：

```bash
cd common/mcp
go test ./...
go vet ./...
```

这些命令验证编译、单元测试、MCP 官方 SDK 适配、RAG 基础测试和前端生产构建，不代表已经完成生产压测或第三方检索基准评测。

## 安全与边界

- 用户、Provider、知识文件、MCP Server 和 MCP Tool 都按账号隔离；服务层会再次校验所有权。
- MCP HTTP 连接默认拒绝 loopback、私网和 link-local 目标，并限制跨源重定向；stdio 不接受 shell 拼接命令。
- 上传文件、模型、ONNX Runtime、`node_modules`、`dist`、二进制和本地 `.env` 都属于运行时数据，不应提交到 Git。
- 当前 RAG 是可运行 MVP，后续可以在独立任务中增加混合检索、重排、父子块和离线评测，不把未经测量的指标写成项目成果。
