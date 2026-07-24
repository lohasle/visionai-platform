# Agent 上下文

Nimbus Framework Go 是面向 App 后台与运营后台的 Go 模块化单体脚手架。

## 不可变边界

- 单个 `nimbus-server` 进程。
- MySQL 8.4。
- `system`、`infra`、`visionai` 为现有功能模块。
- 会员与支付不属于 VisionAI 产品边界，不注册路由、不创建菜单，也不参与数据库初始化。
- `application`、`im`、`app` 只提供 Health 示例。
- 不因对照 Muse 或 Java 工程而自动增加模块、菜单或业务实体。

## 参考关系

`muse-app-go` 可作为已验证的 Go 公共能力参考，但同步时必须保留 Nimbus 包名、品牌、模块和单体进程结构。禁止整仓覆盖。

## 完成定义

代码存在不等于功能完成。只有当前开放菜单对应的接口、数据库行为、前端交互和自动化验证均通过，才可标记完成。
