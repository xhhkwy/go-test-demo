# 分阶段验收记录

所有内容只在新项目内创建。阶段结果只引用实际执行记录；未完成的检查不计为通过。

## 环境

- 宿主机：Windows amd64，Go 1.22.5，CGO_ENABLED=0。
- Docker：Desktop 4.49.0，Engine 28.5.1，Compose v2.40.2；Linux 引擎可访问。
- 生产镜像使用独立固定版本的 Go 构建工具链，最终镜像不携带宿主机工具链。

## 阶段状态

| 阶段 | 状态 |
|---|---|
| 1 基础框架 | 通过 |
| 2 通信与中间件 | 通过 |
| 3 健康与就绪 | 通过 |
| 4 识别引擎 | 通过（Linux 竞态已补验） |
| 5 接口与客户端 | 通过 |
| 6 容器部署 | 通过（正式构建、摘要及运行已验收） |
| 7 最终验收 | 通过 |

## 阶段 1：基础框架（通过）

- 完成：独立 server/client 入口、配置校验、三种数据模型、测试目录。
- 命令：gofmt -w cmd internal；go build -o bin/server.exe ./cmd/server；go build -o bin/client.exe ./cmd/client；go test -v ./...；go vet ./...。
- 实际：两个可执行文件均编译成功；4/4 顶层测试通过，失败 0；vet 无诊断。
- CLI：两个 --help 均退出 0；client --format csv 退出 1，输出配置错误。
- 原始记录：evidence/stage1-tests.txt。
- 未验收：本阶段不包含业务接口和容器运行，按后续阶段执行。

## 阶段 2：通信与中间件（通过）

- 完成：HTTP 传输、默认 30 秒超时、取消、错误处理、请求标识、JSON 日志、请求体与并发限制、缓冲响应和 panic 恢复。
- 命令：go test -v -timeout 30s ./...；go vet ./...。
- 实际：新增 7/7 顶层测试（含 8 个错误响应子场景）通过；100 次 loopback HTTP 往返全部成功；原有 4 项继续通过。
- 初次问题：测试服务未读完 POST 请求体即等待 context，导致测试关闭等待；测试改为先消费请求体并设置等待上界，复测通过。
- 原始记录：evidence/stage2-tests.txt。本阶段无遗留失败；客户端进程退出码在阶段 5 验证。

## 阶段 3：健康与启动就绪（通过）

- 完成：严格规则解析、唯一 ID 与正则/置信度/捕获组校验、启动前加载、健康路由和自带探测命令、10 秒优雅退出。
- 命令：go test -v -timeout 30s ./...；go vet ./...；go build -o bin/server.exe ./cmd/server。
- 实际：新增 5/5 顶层测试通过，含 10 个规则校验子场景、5 种探测响应；累计 16 项顶层测试通过。
- 进程验证：运行 server --addr 127.0.0.1:18080；healthcheck 退出 0，真实 GET /health 返回 {"status":"ok"}；缺失规则启动退出 1；不可访问的探测目标退出 1。
- 记录：evidence/stage3-tests.txt、stage3-health.txt、stage3-server.log、stage3-invalid-start.txt、stage3-unhealthy.txt。
- 环境限制：用户接管镜像仓库连接；后续不再尝试镜像拉取。WSL Ubuntu 未安装 Go/gcc，Linux 竞态检查等待构建工具链可用，不阻塞本地实现。

## 阶段 4：指纹识别引擎（通过）

- 完成：12 条产品/协议规则、4 条 OS 提示、三种解码方式、HTTP 头部隔离、MySQL 截断握手结构解析。
- 命令：go test -v -timeout 30s ./...；go vet ./...；go test ./internal/fingerprint -run '^$' -fuzz '^FuzzRecognize$' -fuzztime 30s -parallel 2；同命令执行 FuzzDecode。
- 实际：20/20 样例逐字段比对通过（前七条与用户给定期望一致）；14/14 识别边界场景通过；编码与外置规则修改测试通过。
- 模糊测试：识别 1,462,592 次，解码 2,083,541 次，均无失败；总计 20 个普通顶层测试和两个模糊入口的种子测试通过。
- 数据规范化：仅第 16 条 TLS 二进制改用 Base64 保留 0xa5/0xa1 等非 UTF-8 字节；MySQL 用标准 JSON 控制字符转义。IIS 留空产品，vsFTPd/Pure-FTPd 只识别 FTP，符合已确认最低范围。
- 记录：evidence/stage4-tests.txt、stage4-fuzz-recognize.txt、stage4-fuzz-decode.txt。
- 初次暂未验收：Linux 竞态检查。当时按用户指示继续独立步骤，后续已在正式 Go 1.27.2 Linux 构建中补验通过。

## 阶段 5：批量接口与客户端（通过）

- 完成：批量 API、严格输入检查、容量限制、记录索引、顺序与重复项、表格/JSON 输出和真实进程测试。
- 命令：go test -count=1 -v -timeout 120s ./...；go vet ./...。
- 实际：累计 27/27 普通顶层测试通过，两个模糊入口的种子通过；真实 server/client 输出 20/20 匹配；20 类 API 错误后均验证 health 200 及正常请求成功。
- CLI：表格含七列；JSON 为数组；缺失文件、非法格式、连接失败、非法本地 JSON 均非零退出并提示原因。
- 修正：输入逐条解码，超过条数上限立即停止，避免海量小记录放大内存。非法 JSON 字符串现在能返回记录索引，更新断言后无缓存复测通过。
- 记录：evidence/stage5-tests.txt。Linux 竞态仍待验收；本阶段无遗留功能失败。

## 阶段 6：容器部署（通过）

- 完成：多阶段 Dockerfile、scratch 双镜像、专用内部网络、HTTP 健康依赖、非 root/只读/能力与资源收紧、调试覆盖、完整自动验收脚本。
- 静态命令：docker compose config --quiet；docker compose -f compose.yaml -f compose.debug.yaml config --quiet。两份配置均通过。
- 运行命令：scripts/runtime-check.ps1；复测使用 SERVER_IMAGE=banner-fingerprint-server:runtime-check、CLIENT_IMAGE=banner-fingerprint-client:runtime-check、FP_DOCKER_TEST=1，执行 go test -count=1 -v -timeout 180s ./integration -run '^TestCompose$'。
- 实际运行：宿主机 Go 1.22.5 交叉编译 Linux 静态程序，从正式 Dockerfile 提取相同 scratch 运行阶段，以 --network none 构建；没有访问镜像仓库。
- 最终结果：1/1 Docker 专项顶层测试通过，5/5 验收组通过，失败 0。包括 20/20 结果、服务 DNS、健康探测时间早于 client 启动、默认无映射、internal 网络、UID/GID 65532、只读挂载、cap_drop ALL、禁止提权、CPU/内存/PID 限制、调试端口、规则修改不重建、SIGTERM 退出 0、坏规则阻止 client 执行。
- 复测修正 1：docker logs 将服务端错误放在 stderr；验收器初次只收集 stdout。已修正日志收集，两种流均用于诊断，复测通过。
- 复测修正 2：实际发现 Docker Desktop 下 internal 网络仅映射端口不可从宿主机访问。仅在 debug 覆盖中为 server 增加调试 bridge，端口保持绑定 127.0.0.1；真实 GET /health 返回 200，默认配置仍只有内部网络。
- 运行镜像：server 约 2.59 MB，client 约 2.33 MB（压缩层大小）；具体 ID 和大小见 evidence/stage6-runtime-images.txt。
- 记录：evidence/stage6-compose-config.json、stage6-debug-config.json、stage6-runtime-check.txt、stage6-runtime-retest.txt、stage6-runtime-final.txt、stage6-runtime-final-retest.txt。前述失败记录保留，最终修复结果以 final-retest 为准。
- 初次暂未验收的正式构建与竞态检查，已按用户后续“你自己构建”的指示完成，见以下正式构建补验。

### 正式构建补验（通过）

- 实际工具链：官方 golang:1.27.2-bookworm；固定摘要 sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61。Dockerfile、Compose、.env.example 和 full-check.ps1 默认一致。
- 命令：scripts/full-check.ps1。执行官方 source → test → build → scratch 完整链路；CGO_ENABLED=1 go test -count=1 -race -timeout 180s ./... 和 go vet ./... 均通过，随后 CGO_ENABLED=0 静态编译两个 Linux 程序。
- 首次正式检查发现 Go 1.22/1.27 对未闭合数组的 decoder.More 行为不同，错误索引断言不一致；统一 SyntaxError/EOF/UnexpectedEOF 为无记录索引的 JSON 语法错误，并新增跨版本回归。Windows 和 Linux 复测均通过。
- 正式镜像再次执行完整 Docker 专项：1/1 顶层测试通过、5/5 验收组通过、20/20 结果匹配；内部访问、真实健康依赖、权限/资源、调试端口、规则修改不重建、优雅退出和坏规则阻止 client 全部通过。
- 正式镜像当次记录：server 3,284,287 字节、client 3,002,709 字节，均以 65532:65532 运行。具体镜像 ID 见 evidence/stage6-production-images.txt。
- 原始证据：evidence/stage6-production-build-first-failure.txt 保留首个失败；stage6-production-build.txt 为修复后构建成功记录；stage6-production-compose.txt 为正式镜像专项通过记录；stage6-full-check-retest.txt 为完整复测输出。
- 为排查下载问题，还从校验 SHA256 的官方 Go 归档自行构建过备用 Linux Go 1.27.2/gcc 工具链镜像，记录在 stage6-toolchain-bootstrap.txt。最终验收使用上述官方 bookworm 固定摘要镜像。

## 阶段 7：最终回归（通过）

- 完成：README、20 条标准化输入及预期、独立 Windows 可执行文件、完整验收脚本及题目映射。
- 命令：go test -count=1 -json -timeout 120s ./...；go vet ./...；gofmt -l cmd internal integration；两个程序 --help。
- 实际：30 个顶层测试入口通过（28 个普通测试、2 个模糊种子入口）；含子场景共 110 项通过，0 项失败；默认跳过 1 个需显式启用的 Docker 专项。该 Docker 专项已在阶段 6 使用正式镜像单独执行并通过，故不把 skip 误计为失败或执行成功。
- 记录：evidence/final-tests.jsonl、final-test-summary.json。两个二进制已从当前实现重新编译；vet 无诊断、gofmt 无差异。
- 一键启动实测：在隔离测试项目执行 docker compose up --build -d，server 显示 healthy，client 输出 20 条结果并 Exited (0)；默认无宿主机端口，随后清理测试容器与网络。记录：evidence/stage7-oneclick.txt、stage7-oneclick-results.txt、stage7-oneclick-status.txt。
- 交付仓库：https://github.com/xhhkwy/go-test-demo.git 。所有软件必达项验收通过后执行提交与推送，交付提交详情以仓库 Git 历史为准。

### 最终题目对应

| 题目必达项 | 结果 | 证据 |
|---|---|---|
| Go 独立 client/server | 通过 | 进程测试真实编译并启动两个程序 |
| POST /fingerprint、GET /health | 通过 | API 测试、进程测试、容器实际探测 |
| 七字段、顺序、版本/OS/置信度 | 通过 | 20 条逐字段对比，前七条为用户给定期望 |
| SSH、HTTP nginx/Apache/Jetty、MySQL、Redis、FTP | 通过 | golden 测试和 Docker client 的 20 条输出 |
| unknown 与畸形 banner 不崩溃 | 通过 | TLS/QUIT/空值/边界测试、约 354 万次模糊执行、错误后恢复测试 |
| 容器访问收敛、真实健康依赖、权限收紧 | 通过 | Docker inspect、健康时间顺序、启动失败测试 |
| 规则与程序解耦 | 通过 | 外置规则修改后相同镜像重启并验证结果改变 |
| Docker Compose 正式源码一键构建启动 | 通过 | 正式构建、专项测试、真实 up --build 与 client 退出 0 |
| Linux 竞态检查、构建镜像摘要 | 通过 | Go 1.27.2 正式构建日志、默认固定 SHA256 |

整体结论：七阶段完成；功能、识别深度、异常稳定性、正式镜像构建、Linux 竞态检测、真实健康依赖、容器权限及规则解耦均通过，整体验收通过。
