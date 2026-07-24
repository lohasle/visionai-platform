# SPEC-1300 数据资产中心与 S3 存储

状态：已完成
Linear：LOH-8

## 范围

覆盖 FR-DATA-001..008：分片/断点上传、S3/目录/ZIP/JSONL 导入、元数据、SHA256/去重、质量检查、标签/Collection、引用关系、回收站。对象存储只通过 StorageProvider。

## 规则

- 对象根路径为 `tenants/{tenantId}/projects/{projectId}`，业务库只存 URI、摘要、大小和元数据。
- 上传先建 UploadSession；临时对象和 DB 失败均有补偿；冻结版本引用阻止物理删除。
- 单文件至少 10GB；列表分页和索引支持单项目百万元数据基线。

## 验收

- E2E-CV-02：100 张图全部生成资产、缩略图、哈希和导入报告。
- 上传可恢复；重复文件策略可配置；损坏/缺失对象有明确状态和修复动作。
- 路径穿越、伪造 Content-Type、越权签名 URL 和 Secret 泄漏测试通过。

## 实现与证据

- 实现 UploadSession/Chunk/Complete、目录/ZIP/JSONL 导入、SHA-256 去重、标签、集合、质量和回收站。
- 真实导入 100 张唯一 PNG，100/100 成功，建立缩略图、哈希、对象 URI 和导入报告。
- StorageProvider 统一 MinIO 访问；预签名端点区分容器内部与浏览器公开地址；路径规范化和项目授权测试通过。
