# SPEC-2210 GitHub Pages UI/UX Pro Max 重设计

状态：已完成

Linear：LOH-18

## 背景

GitHub Pages 已具备真实产品截图和全链路内容，但需要更鲜明的企业产品气质、更完整的视觉证据和更稳定的响应式体验。本次使用 UI/UX Pro Max 重新定义页面设计系统与信息架构。

## 范围

- 采用 Swiss Modernism 2.0、编辑网格和产品演示结合的设计语言。
- 首屏直接呈现真实工作台、公开验收数据和明确的产品定位。
- 按数据与标注、训练与评估、模型发布与反馈组织完整产品叙事。
- 19 张验收截图全部进入可筛选证据库，支持键盘操作和原图预览。
- 去除渐变、光晕、伪背书、通用 SaaS 卡片堆叠和模板化 AI 文案。
- 保持纯静态 HTML/CSS/JavaScript，不引入运行时依赖。

## 非目标

- 不修改 VisionAI 业务前后端、数据库、Compose 服务或线上账号。
- 不新增会员、支付、定价和营销线索收集能力。
- 不伪造客户案例、业务指标或第三方认证。

## 验收

- 页面源文件 UTF-8，无 `????`、Unicode 替换字符或乱码。
- 375、768、1024、1440px 下无水平滚动，核心内容顺序正确。
- 主导航、筛选器、截图预览和关闭动作均可使用键盘完成。
- 所有真实截图声明尺寸；首屏图优先加载，其余图片懒加载。
- `prefers-reduced-motion` 下禁用非必要过渡。
- GitHub Pages workflow 成功，线上页面与本地构建一致。

## 实现与证据

- 设计系统：`design-system/visionai/MASTER.md` 与 `design-system/pages/github-pages.md`。
- 页面实现：`docs/site/index.html`，19 张真实验收截图全部进入分类证据库。
- GitHub：PR [#5](https://github.com/lohasle/visionai-platform/pull/5)，CI `verify` 通过。
- HTML：`npx --yes html-validate docs/site/index.html` 零错误，`git diff --check` 通过。
- 浏览器：375、768、1024、1440px 均无横向溢出；375px 可见交互目标均不小于 44×44px。
- 交互：证据筛选、Lightbox 打开、Escape 关闭、焦点进入与回归均通过。
- 资源：19/19 截图与产品手册 HTTP 200；控制台零错误、零警告；页面无 `????` 或 Unicode 替换字符。
- 发布地址：https://lohasle.github.io/visionai-platform/
