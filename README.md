# Banner 指纹识别系统

Go 实现的离线 Banner 识别服务，包含独立 server 和 client。只分析提供的 ip、port、banner，不向这些目标发起连接。

## 一键启动

需要 Docker Engine（Linux 容器）和 Docker Compose v2。在本项目目录执行：

```powershell
docker compose up --build
```

server 完成规则加载并通过 HTTP 健康检查后，client 读取挂载的 20 条样例并打印七列表格，成功退出；server 继续运行。默认没有宿主机端口映射。后台运行和查看结果：

```powershell
docker compose up -d --build
docker compose logs client
docker compose ps --all
docker compose down
```

默认构建镜像为 `golang:1.27.2-bookworm`，已在 Dockerfile、Compose 及构建脚本中固定其 SHA256 摘要，避免标签漂移。可用 `GO_IMAGE` 指定准备好的同版本镜像或 `名称@sha256:摘要`。最终镜像是 scratch，不包含 Go 工具链、shell、curl 或包管理器。实际构建、竞态检查和分阶段验收状态见 [验收记录](docs/acceptance.md)。

## 替换输入和调用客户端

输入文件必须是 UTF-8 标准 JSON 数组。目录和文件均只读挂载；输入文件必须已存在。PowerShell 示例：

```powershell
$env:INPUT_FILE = 'E:/path/to/scans.json'
docker compose run --rm client --input /data/scans.json --server http://server:8080 --format json
```

未启动 server 时，`compose run` 会按健康依赖启动它。容器内始终通过服务名 `server:8080` 通信。客户端支持：

| 参数 | 默认值 |
|---|---|
| --input | testdata/scans.json（Compose 中为 /data/scans.json） |
| --server | http://127.0.0.1:8080（Compose 中为 http://server:8080） |
| --format | table，可选 json |
| --timeout | 30s |

输出写入 stdout，错误写入 stderr。成功（包括 unknown）退出 0；输入、连接、服务端或输出错误退出 1。一个文件作为一个批次提交，超限时需拆分文件。

## 本机编译和运行

最低支持 Go 1.22；本地不需要额外 Go 依赖。生产构建使用 Dockerfile 指定的独立工具链。

```powershell
go build -o bin/server.exe ./cmd/server
go build -o bin/client.exe ./cmd/client
./bin/server.exe --addr 127.0.0.1:8080 --rules rules/fingerprints.json
# 另一个终端：
./bin/client.exe --input testdata/scans.json --format json
./bin/server.exe healthcheck --url http://127.0.0.1:8080/health
```

Linux/macOS 构建时去掉二进制的 .exe 后缀。健康探测实际检查 HTTP 状态码和 JSON 中的 status，失败时非零退出。

## HTTP 接口

### POST /fingerprint

Content-Type 为 `application/json`，可带 charset 参数；请求与响应均是顶层数组：

```json
[
  {"ip":"1.2.3.4","port":22,"banner":"SSH-2.0-OpenSSH_8.9p1 Ubuntu-3"},
  {"ip":"1.2.3.7","port":3306,"banner":"J\u0000\u0000\u0000\n8.0.32\u0000"}
]
```

```json
[
  {"ip":"1.2.3.4","port":22,"protocol":"SSH","product":"OpenSSH","version":"8.9p1","os_hint":"Ubuntu","confidence":0.95},
  {"ip":"1.2.3.7","port":3306,"protocol":"MySQL","product":"MySQL","version":"8.0.32","os_hint":"","confidence":0.9}
]
```

响应保留输入 IP、端口、顺序和重复项，七个字段始终存在。协议值为 SSH、HTTP、MySQL、Redis、FTP 或 unknown。识别出 HTTP 但未识别产品时，保留 HTTP，产品和版本为空；443 端口上的明文 HTTP 响应同样输出 HTTP。缺少版本/OS 证据时留空，不根据端口推测。

可选字段 `banner_encoding`：

| 值 | 解释 |
|---|---|
| text 或省略 | JSON 解码后的字符串直接作为 banner；不再次处理字面量反斜杠 |
| escaped | 在 JSON 解码后，再还原一次 `\xHH`、`\n`、`\r`、`\t`、`\\`、`\"` |
| base64 | 标准 Base64，适合任意二进制字节 |

合法 JSON 中保存扫描工具的字面量十六进制转义：

```json
[{"ip":"1.2.3.7","port":3306,"banner":"J\\x00\\x00\\x00\\n8.0.32\\x00","banner_encoding":"escaped"}]
```

JSON 字符串直接写 `\x00` 不合法。给定的 MySQL 样例已转为标准 JSON 控制字符转义；TLS 样例用 Base64 保留原始非 UTF-8 字节。没有修复重复引号或非标准 JSON 的隐式行为。

默认限制：8 MiB 请求体、1000 条记录、64 KiB 解码后单条 banner。server 可用 `--max-body-bytes`、`--max-batch`、`--max-banner-bytes` 调整；客户端沿用默认限制。IPv4/IPv6 均支持，端口为 1～65535，必需字段不能缺失或为 null。

| 状态 | 含义 |
|---|---|
| 200 | 批量成功；未知/空/不完整 banner 正常返回 unknown |
| 400 | JSON、字段、IP、端口或指定编码无效；整个批次拒绝 |
| 413 | 请求体、批次数量或解码后 banner 超限 |
| 415 | Content-Type 无效 |
| 503 | 超出四个并行识别请求 |
| 504 | 请求取消或处理期限已到 |
| 500 | 未预期的内部错误，经恢复后返回统一错误 |

错误结构：

```json
{"error":{"code":"invalid_input","message":"port must be between 1 and 65535","index":0,"request_id":"..."}}
```

index 是从 0 开始的记录索引；无法定位记录的错误不含 index。响应头包含 X-Request-ID。原始 banner 不进入服务端日志。

### GET /health

规则成功加载且服务就绪时返回 `200 {"status":"ok"}`；退出排空阶段返回 503。规则读取或编译失败时直接退出，不开启监听。

## 识别规则

默认识别 OpenSSH、nginx、Apache、Jetty、MySQL、Redis、ProFTPD。FTP 其他产品只输出明确可识别的协议；IIS 样例只输出 HTTP。TLS 片段和 QUIT 输出 unknown。20 条预期见 `testdata/expected.json`，前七条遵循题目给定结果。

配置为 `rules/fingerprints.json`，包含 version=1、rules 和 os_hints。默认以只读目录挂载到 /rules。可设置 RULES_DIR 替换规则目录，文件名仍为 fingerprints.json。修改后执行：

```powershell
docker compose restart server
docker compose run --rm client
```

规则字段：

| 字段 | 含义 |
|---|---|
| id | 全文件唯一标识 |
| source | banner、ssh、http_status、http_server、mysql_version 或 ftp |
| pattern | Go regexp（RE2）正则；启动时预编译 |
| protocol / product | 命中时的固定输出 |
| version_group | 版本捕获组索引；0 表示不提取 |
| priority | 降序，数值相同时按 ID 升序；首次命中结束 |
| confidence | 0～1 的规则权重，遵循样例 |

OS 提示包含 id、source、pattern、hint，按配置顺序取第一个匹配项。HTTP 产品和 OS 仅根据响应头提取，不扫描正文；MySQL 解析器验证包头和完整的 NUL 结束版本字符串，允许后续握手内容缺失。规则文件上限 1 MiB、总规则数不超过 1000；非法规则阻止启动。结构解析在 Go 中，产品特征和输出在外置 JSON 中。

## 部署设置

- 专用 internal 网络；默认无宿主机映射，只有 client/server 加入。
- server 的健康检查调用自身二进制，无需 shell；client 使用 service_healthy 等待真实就绪。
- UID/GID 65532，根文件系统及输入/规则挂载只读，cap_drop ALL，no-new-privileges。
- server 512 MiB、client 128 MiB，两者各 1 CPU 和 64 PID；日志 10 MiB × 3。
- server 读头 5 秒、读取 15 秒、写入 30 秒、空闲 60 秒，业务上下文 25 秒；SIGTERM 优雅退出最多 10 秒，Compose 留 15 秒。
- 构建顺序为 source → test（竞态测试与 vet）→ build（静态编译）→ scratch。构建上下文白名单仅包含本项目必要源码、规则和测试数据。

需要从宿主机调试 API 时：

```powershell
docker compose -f compose.yaml -f compose.debug.yaml up -d --build
Invoke-RestMethod http://127.0.0.1:8080/health
```

该覆盖文件额外将 server 加入调试 bridge 网络，使 Docker Desktop 上的宿主机端口可达；端口仅绑定 127.0.0.1，可用 DEBUG_PORT 改端口。默认配置仍只有 internal 网络。关闭时使用相同的 -f 参数执行 down。

## 测试与验收

```powershell
go test -count=1 -v -timeout 120s ./...
go vet ./...
go test ./internal/fingerprint -run '^$' -fuzz '^FuzzRecognize$' -fuzztime 30s -parallel 2
go test ./internal/fingerprint -run '^$' -fuzz '^FuzzDecode$' -fuzztime 30s -parallel 2
```

Go 进程测试会实际构建并启动两个程序，断言 20 条结果、表格和 CLI 失败退出码。使用 -count=1 确保外部构建进程也重新执行。Docker 专项测试默认明确跳过，正式构建后运行：

```powershell
./scripts/full-check.ps1
```

测试使用独立临时 Compose 项目和临时规则，覆盖真实健康启动依赖、20 条结果、权限与资源限制、内部网络、规则改动不重建、优雅退出、损坏规则阻止 client 启动；结束后清理测试容器和网络。

仓库暂不可用时，可运行 `./scripts/runtime-check.ps1`：本地 Go 交叉编译 Linux 静态程序，从同一 Dockerfile 提取 scratch 运行阶段，使用 --network none 构建补充验收镜像，再运行同一 Compose 测试。该检查仅验证容器运行行为；正式多阶段构建和 Linux 竞态测试的结果仍单独记录。
