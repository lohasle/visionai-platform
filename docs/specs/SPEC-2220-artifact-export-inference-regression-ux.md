# SPEC-2220 训练产物导出、推理回归测试与产品体验完善

状态：已完成

Linear：LOH-19

## 背景

训练实验已经能够归集日志、权重和结果清单，部署模块也已经能够基于已有资产执行在线推理，但用户还缺少两个完成任务的关键出口：将训练产物带出平台，以及直接上传图片验证当前在线模型。部署页也需要把“服务存在”和“现在可以推理”区分开来。

GitHub Pages 同时从项目验收页调整为面向算法工程师和平台负责人的产品介绍页，使用真实工作界面解释用户可以完成的工作，不再采用“运行证据”“验收证明”等工程自证式叙事。

## 用户故事

- 作为算法工程师，我可以下载单个训练产物，也可以将一次训练的元数据、指标、结果清单和全部可下载产物导出为 ZIP。
- 作为发布负责人，我可以在部署详情中看到当前修订、就绪状态、最近一次推理和近期错误率。
- 作为测试人员，我可以上传一张图片，设置预期标签与最低置信度，并获得可留痕的通过/失败结果。
- 作为标注员或审核员，我使用 VisionAI 底座账号直接进入 CVAT，不再输入第二套账号密码；每个操作仍归属到本人。
- 作为数据负责人，我可以从项目成员中一次选择多名标注员和审核员，平台自动创建独立 CVAT 身份并把分片 Job 分配给对应人员。
- 作为评估人员，我使用 VisionAI 当前会话直接进入受保护的 FiftyOne 数据集，不需要额外登录。
- 作为产品访客，我可以从 GitHub Pages 快速理解平台覆盖的数据、训练、评估、发布和生产反馈流程。

## API 契约

### 训练产物

- `GET /api/v1/visionai/projects/{projectId}/training-runs/{runId}/artifacts/{artifactId}/download`
  - 校验租户、项目、运行和产物归属。
  - 仅允许读取当前训练运行对象前缀下的 `s3://` 对象。
  - 返回附件流，保留媒体类型、文件名与长度。
- `GET /api/v1/visionai/projects/{projectId}/training-runs/{runId}/export`
  - 返回 `application/zip`。
  - 包含 `training-run.json`、可用的 `result-manifest.json`、训练产物和日志。
  - 外部 Provider URI 记录在元数据中，但不由平台代理抓取。

### 部署状态与图片回归

- `GET /api/v1/visionai/projects/{projectId}/deployments/{deploymentId}`
  - 增加 `inferenceStatus`，包含 `ready`、部署/修订状态、当前修订、模型版本、最近请求与最近错误。
- `POST /api/v1/visionai/projects/{projectId}/deployments/{deploymentId}/predict-image`
  - `multipart/form-data`：`file` 必填，`expectedLabel` 和 `minimumConfidence` 可选。
  - 允许 JPEG、PNG、WebP；文件最大 20 MiB，像素不超过 40 MP。
  - 仅 `RUNNING` 且存在当前修订的部署可以执行。
  - 返回推理结果、Trace ID 和 `NOT_ASSERTED / PASSED / FAILED` 回归状态。

### 工作台单点登录与统一身份

- `POST /api/v1/visionai/projects/{projectId}/annotation-tasks/{taskId}/workbench`
  - 当前用户必须是项目成员，并且属于任务标注员、审核员或项目负责人。
  - 按需将底座 `AdminUser` 自动映射为独立 CVAT 用户，不接受浏览器提交的外部用户 ID、用户名或密码。
  - 返回 90 秒有效、只可使用一次的 CVAT 工作台地址；票据交换后写入当前用户的 CVAT Session Cookie，再跳转到目标 Task/Job。
- `POST /api/v1/visionai/projects/{projectId}/evaluation-runs/{runId}/workbench`
  - 返回一次性 FiftyOne 工作台地址；交换后生成短期、HttpOnly 的工作台会话。
  - FiftyOne 网关对页面、静态资源和 WebSocket 统一执行会话校验，未授权访问返回 401。
- `GET /api/v1/visionai/workbench-sso/cvat`
- `GET /api/v1/visionai/workbench-sso/fiftyone`
- `GET /api/v1/visionai/workbench-sso/fiftyone/validate`
  - 以上为工作台网关调用的公开交换/校验端点；票据使用 SHA-256 摘要落库，过期或重复消费均拒绝。

### 多人协同标注

- 标注任务的 `annotatorIds`、`reviewerIds` 必须来自同租户的启用底座用户，并且是当前项目成员；除项目负责人外，分别要求 `ANNOTATOR`、`REVIEWER` 项目角色。
- 创建任务时自动同步外部身份；数据上传 CVAT 后按标注员轮询分配 Jobs，保证不同用户可同时在线处理不同分片。
- 进入标注阶段时重新分配给标注员，进入审核阶段时重新分配给审核员，驳回后回到原标注员。
- CVAT 密码使用随机值生成并以 AES-GCM 加密保存，仅用于服务端换取会话，不通过 API、日志或前端暴露。

## 数据模型

`InferenceTrace` 增加：

- `sourceType`：`ASSET`、`UPLOAD` 或 `VIDEO_UPLOAD`。
- `sourceName`、`sourceSha256`：上传文件名称与内容摘要。
- `testMode`：普通在线测试或 `REGRESSION`。
- `expectedLabel`、`minimumConfidence`、`regressionStatus`、`matchedDetectionCount`。

已有记录以 `ASSET` 和 `NOT_ASSERTED` 作为兼容默认值。

`CVATUserMapping` 增加仅服务端可见的加密凭据字段；`WorkbenchTicket` 保存 Provider、租户、项目、用户、资源、重定向地址、票据摘要、过期时间与消费时间。

## 交互与视觉

- 训练运行成功后在列表和详情提供导出；产物表提供单项下载、大小和不可下载原因。
- 部署详情顶部显示独立的推理就绪栏；每五秒刷新并在离开页面时释放轮询。
- 图片测试是默认入口，资产 ID 测试作为次级入口；上传区包含格式/大小提示、文件替换和断言字段。
- 所有异步按钮提供 loading/disabled，错误就近呈现并提供重试路径；空数据使用明确空状态。
- 可点击部署项使用原生按钮语义，焦点环可见，交互目标不小于 44px。
- GitHub Pages 使用 Swiss Modernism 2.0 编辑网格、真实产品截图和任务型文案；移除渐变、光晕、伪背书、AI 套话和“证据/验收”标题。
- 标注任务创建使用底座用户多选控件，显示项目角色和外部身份同步状态，不再要求用户手填平台用户 ID 或 CVAT ID。
- 内嵌工作台加载前显示“正在建立个人会话”，票据失败、会话过期和 Provider 不可用均提供重试或返回平台的恢复路径。

## 安全与审计

- 服务端不信任客户端提供的 URI、文件名、媒体类型或运行归属。
- 对象 Key 必须位于当前租户、项目与训练运行前缀内，拒绝跨项目读取和路径穿越。
- 图片先限流读取，再做格式和像素校验；不将回归上传保存为数据资产。
- 下载、导出和回归测试写入审计日志；审计失败不泄露存储凭证或内部错误。
- 工作台票据只保存摘要、90 秒过期且单次消费；重定向地址必须属于配置的 CVAT/FiftyOne Public URL。
- FiftyOne 不再直接暴露未鉴权 UI；所有浏览器流量先经过工作台网关。
- 标注任务与 CVAT Job 权限由底座租户、项目成员和项目角色派生，禁止通过手工外部映射绕过。

## 验收

- 单个训练产物和完整 ZIP 均可由浏览器下载，ZIP 可正常解压且元数据完整。
- 非本项目、非本运行、外部 URI 或不存在的对象返回明确错误。
- 上传有效图片可以推理；不支持格式、超大文件、超大像素和未就绪部署会被拒绝。
- 回归断言的通过、失败和未断言三种状态都有单元测试与界面呈现。
- OpenAPI、路由契约、后端测试、前端类型检查/构建和真实浏览器流程通过。
- GitHub Pages 在 375、768、1024、1440px 无横向滚动、无乱码，页面文案不再出现“运行证据/验收证明”。
- 两个不同的底座标注账号可以同时进入同一任务的不同 CVAT Job，浏览器无二次登录，CVAT 会话用户名彼此独立。
- 非任务成员、角色不匹配用户、停用底座用户、过期/重复票据均无法进入 CVAT；FiftyOne 未经票据不能直接访问。
- CVAT/FiftyOne 均可在 VisionAI 全屏内嵌工作台与外部新窗口中打开，刷新后会话仍有效，退出 VisionAI 后新票据不可签发。
- 更新真实产品截图、PR、CI、GitHub Pages 和 Linear 状态。
