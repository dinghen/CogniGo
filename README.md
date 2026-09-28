# CogniGo

我开发并维护 CogniGo，用它练习 Go 服务端分层、AI 应用集成和本地可复现的工程流程。项目提供账号注册与登录、会话和 AI 对话、知识文件上传与检索，以及一个独立运行的 MCP 工具服务。

## 我实现的内容

- 使用 Go 和 Gin 提供 REST API；AI 对话支持普通响应与 SSE 流式响应，用户接口由 Controller、Service、DAO 分层组织。
- 使用 CloudWeGo Eino 接入聊天模型、Embedding 和 RAG 检索。当前知识库按 Markdown 标题、段落等结构切片，默认上限为 1000 个 Unicode 字符、重叠 150 个字符；这两个值是字符数，不是 Token 数。
- 使用 Redis Stack 的向量索引存储和检索知识片段；保留上传文件作为原始数据，检索结果携带来源信息，并在无相关资料时明确提示模型不要编造。
- 使用官方 Go MCP SDK 实现独立 MCP 服务和客户端；客户端初始化并发现工具，将 MCP 工具映射到 Eino 原生工具调用，再把结构化结果交回模型。
- 使用 MySQL/GORM 保存用户、会话和消息；RabbitMQ 用于异步消息持久化；Redis 还用于验证码和短期状态；接口通过 JWT 保护。
- 使用 Docker Compose 启动 MySQL、Redis Stack、RabbitMQ 和本地邮件收件箱。图像识别依赖未纳入仓库的 ONNX Runtime 与模型文件。

Embedding 服务需要兼容当前 Eino Ark 适配器使用的 OpenAI-compatible Embeddings API。切换模型时不仅要改模型标识，还要配置兼容的 endpoint 和凭据；向量维度由运行时的 Embedding 响应探测，Redis 索引按实际维度建立。模型或 endpoint 改变后，原始上传文件需要重新向量化，Redis Stack 服务本身不需要更换。

当前 RAG 是面向单用户单文件知识库的 MVP，没有实现混合检索、重排模型、多文件管理或语义模型切片。我还没有建立带人工标注问题的检索评测集或独立性能基准，因此不把 Recall、MRR、P95 延迟和吞吐量写成项目成果。

## 本地运行

依赖 Go 1.25+、Node.js 22 LTS、npm 和 Docker Compose。复制环境变量模板，并至少填写聊天/Embedding 服务的 API Key：

```bash
cp .env.example .env
# 编辑 .env，填写 OPENAI_API_KEY；模型服务按需配置。
make setup
make frontend-install
make infra-up
```

本地注册默认通过 Mailpit 接收邮件，不需要真实邮箱或 SMTP 授权码。启动后打开 `http://127.0.0.1:8025` 查看验证码和系统发出的登录账号。注册成功后页面会使用服务端返回的 JWT 直接进入应用；之后也可以用 Mailpit 中收到的账号和注册密码登录。Mailpit 端口只绑定到本机回环地址。

分别在终端启动 API、MCP 服务和前端：

```bash
make run
make run-mcp
make frontend-dev
```

默认访问地址：

| 服务 | 地址 |
| --- | --- |
| Web 前端 | `http://127.0.0.1:8080` |
| Go API | `http://127.0.0.1:9090` |
| MCP HTTP 服务 | `http://127.0.0.1:8081/mcp` |
| Mailpit 收件箱 | `http://127.0.0.1:8025` |
| MySQL / Redis Stack | `127.0.0.1:3306` / `127.0.0.1:6379` |
| RabbitMQ / 管理界面 | `127.0.0.1:5672` / `http://127.0.0.1:15672` |

如果本机端口被占用，可在 `.env` 中覆盖对应的 `COGNIGO_*_PORT`；前端端口可通过 Vue CLI 的 `--port` 参数覆盖。`make infra-up` 只启动基础设施，不会自动启动 API 或前端。

运行 RAG 和模型对话需要有效的 `OPENAI_API_KEY`。默认配置指向阿里云百炼的 OpenAI 兼容接口；切换供应方时同时设置 `COGNIGO_RAG_BASE_URL` 和 `COGNIGO_EMBEDDING_MODEL`，并确认新服务提供兼容的 Embeddings 接口。索引会按运行时取得的维度创建；更换模型后首次检索会从本地保留的上传文件重建向量。不要手动填写 Redis 向量维度。

图像识别模型和 ONNX Runtime 原生库不随 Git 仓库发布，需要时执行：

```bash
make models
```

## 验证

```bash
make test
make vet
make build
make build-mcp
make frontend-build
```

以上是本地可运行与自动化检查范围，不代表已进行生产压测或第三方 RAG 基准评测。`.env`、上传文件、下载模型和构建产物均为本地数据，不应提交到 Git。
