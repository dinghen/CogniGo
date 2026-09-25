# CogniGo v2 本地运行

本目录是仓库当前的第二版实现。后端会自动执行 MySQL 表迁移；Redis 必须使用 Redis Stack（包含 RediSearch），RabbitMQ 用于异步保存聊天消息。

推荐使用 Go 1.24.10 或更高版本、Node.js 22 LTS、npm 和 Docker Desktop（WSL 2 集成）。当前代码也已在 Go 1.26 上通过构建；Node.js 24 可以构建前端，但旧版 Vue CLI 依赖会显示兼容性警告。

## 1. 初始化

```bash
cp .env.example .env
# 编辑 .env，至少填写 OPENAI_API_KEY；按需填写邮箱和百度 TTS 密钥。
make setup
make frontend-install
```

`.env` 中的邮箱授权码、百度 API 密钥和 JWT 密钥属于敏感信息，不要提交到 Git。阿里百炼兼容 OpenAI API，文档中使用的变量名是 `OPENAI_API_KEY`、`OPENAI_MODEL_NAME` 和 `OPENAI_BASE_URL`。

图像识别模型和 ONNX Runtime 原生库不随 Git 仓库提交，首次需要时执行：

```bash
make models
```

脚本会根据 Linux/macOS 和 CPU 架构下载 ONNX Runtime 1.22.0、MobileNetV2 以及 ImageNet 标签。默认 Linux x86_64 运行库路径是 `.local/onnxruntime/lib/libonnxruntime.so.1.22.0`，可通过 `.env` 中的 `COGNIGO_ONNX_LIBRARY_PATH` 覆盖。

## 2. 启动基础设施

Docker Desktop 或 Docker Engine 启动后执行：

```bash
make infra-up
```

在 WSL 中使用 Docker Desktop 时，需要先在 Docker Desktop 设置里启用对应发行版的 WSL Integration。可运行 `make doctor` 检查本机命令、Docker daemon、`.env` 和模型文件。

Compose 会启动：

| 服务 | 本机端口 | 用途 |
| --- | ---: | --- |
| MySQL 8.0 | 3306 | 用户、会话和消息持久化 |
| Redis Stack | 6379 | 验证码、缓存和 RAG 向量索引 |
| RabbitMQ | 5672 / 15672 | 异步消息；15672 是管理界面 |

默认开发凭据与 `.env.example` 一致。生产环境请在 `.env` 中改掉密码，并限制端口暴露范围。

## 3. 启动服务

在 `CogniGo-v2` 目录分别打开终端：

```bash
make run-mcp       # MCP 天气工具，监听 8081
make run           # Go API，监听 9090
make frontend-build # 生产构建（开发调试用 npm run serve）
```

开发前端（会读取 `.env` 中的 `COGNIGO_API_URL` 作为后端代理地址）：

```bash
make frontend-dev
```

浏览器访问 `http://127.0.0.1:8080`。前端开发服务器会把 `/api` 代理到 `http://127.0.0.1:9090`。

## 4. 验证

```bash
make test
make vet
make build
make build-mcp
```

真正启动后端前，必须先让 MySQL、Redis Stack 和 RabbitMQ 处于健康状态。MCP 模型还需要 `make run-mcp`，RAG 模型还需要有效的阿里百炼 Key 和 Redis Stack 向量索引。

本地 `.env` 已被 Git 忽略。仍需由使用者填写的凭据是：阿里百炼 `OPENAI_API_KEY`、QQ 邮箱账号及 SMTP 授权码、百度 TTS API Key 和 Secret Key。不要把这些值写入 `config.toml` 或提交到仓库。
