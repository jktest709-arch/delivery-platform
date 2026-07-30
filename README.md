# 统一交付平台

基于 GitLab CI 的私有化交付平台。当前版本已从前端状态原型重构为前后端工程：Go 后端负责权限、配置、上线单、发布历史和 GitLab API 编排；Vue 管理台负责上线申请、范围打 tag 构建、单项目重试和配置管理。

## 技术栈

- 后端：Go、Gin、GORM、JWT，按 go-admin 常见后台工程分层组织。
- 前端：Vue 3、Vite、TypeScript，作为 go-admin-ui 风格的管理台二开入口。
- 数据库：本地默认 SQLite，私有化部署默认 MySQL 8.4。
- GitLab：后端统一托管 GitLab Token，支持 dry-run 和真实 API 模式。
- 部署：Docker Compose，包含 MySQL、后端 API、前端 Nginx。

## 项目结构

```text
backend/        Go API 服务，包含数据模型、接口、GitLab 客户端和发布编排
frontend/       Vue 管理台，调用后端 API，不再使用浏览器临时状态
deploy/         Nginx 配置和环境变量示例
docs/           架构与接口说明
app/            早期 Next/Vinext 交互原型，保留作页面参考
```

## 本地开发

当前机器需要先安装 Go 1.22+。

```bash
# 1. 启动后端，默认 SQLite + GitLab dry-run
cd backend
go mod tidy
go run ./cmd/server

# 2. 另开终端启动前端
cd frontend
npm install
npm run dev
```

打开 `http://localhost:5173`。

## 自动化测试

```bash
# 后端接口契约和单元测试
cd backend
go test ./...

# 前端构建和组件测试
cd frontend
npm ci
npm run build
npm run test

# 登录和主页面冒烟 E2E
npx playwright install chromium
npm run test:e2e
```

当前测试覆盖：

- `/api/projects` 契约：`dependencies` 必须返回数组，不能返回 `null`。
- 前端登录后渲染：接口返回 `dependencies: null` 时不能白屏。
- E2E 冒烟：`admin/admin123` 登录后切换所有主页面，浏览器不能出现运行时错误。
- 用户权限与 Pipeline 流程：覆盖管理员用户管理入口，以及执行台内 Tag、Pipeline、构建/部署 jobs 展示。

默认账号：

```text
admin / admin123       管理员
release / release123   发布经理
dev / dev123           开发
```

## 私有化部署

```bash
cp deploy/.env.example .env

# 按需修改 .env：
# JWT_SECRET=强随机字符串
# WEB_PORT=8080
# GITLAB_BASE_URL=https://gitlab.your-company.com
# GITLAB_TOKEN=你的 GitLab Token
# GITLAB_DRY_RUN=true

docker compose up --build -d
```

部署完成后访问 `http://服务器IP:8080`。后端健康检查为 `http://服务器IP:8080/healthz`。

如果希望改成 `18080` 访问，不要只改 `HTTP_ADDR`，而是这样配置：

```text
WEB_PORT=18080
CORS_ORIGINS=*
```

`WEB_PORT` 是宿主机对外访问端口；`HTTP_ADDR` 是后端容器内部监听地址，通常保持 `:8080` 即可。修改 `.env` 后需要重新创建容器：

```bash
docker compose down
docker compose up --build -d
```

企业内网如果拦截 HTTPS，Docker 构建时可能出现 Go/npm 依赖下载证书错误。处理方式是给 Docker 构建环境导入企业根证书，或指定可访问的 Go 代理：

```bash
docker compose build --build-arg GOPROXY=https://goproxy.cn,direct backend
docker compose up -d
```

## GitLab 真实模式

默认 `GITLAB_DRY_RUN=true`，点击范围打 tag 构建、单项目重试、部署时只会写入平台数据库，不会调用真实 GitLab。

切换真实模式：

```text
GITLAB_DRY_RUN=false
GITLAB_BASE_URL=https://gitlab.your-company.com
GITLAB_TOKEN=<Personal Access Token 或 Project Access Token>
```

`GITLAB_BASE_URL` 填 GitLab 站点根地址即可，例如 `https://gitlab.your-company.com`，不要填具体项目仓库地址。即使误填成 `https://gitlab.your-company.com/api/v4`，后端也会自动兼容为站点根地址。

Token 至少需要具备创建 tag、读取 pipeline/jobs、触发 manual job 的权限。项目配置里的 `GitLab Project ID` 支持数字 ID，也支持 `group/project` 路径；这里不要填写 `https://gitlab.../group/project` 这种完整仓库 URL。

如果统一打 tag 返回 `403 Forbidden`，说明已经进入真实 GitLab 调用，但当前 Token 被 GitLab 拒绝创建 tag。优先检查：

- Token 对应用户是否至少拥有该项目创建 tag 的权限，通常建议使用 Maintainer 或项目级 Access Token。
- Token scope 是否包含 API 调用权限。
- GitLab 项目 `Settings -> Repository -> Protected tags` 是否配置了 `ftprd-*`、`aaprd-*` 等受保护 tag 规则，并且允许该用户/角色创建。
- 上线单里选择的源 ref 是否正确；如果是从分支或 commit 发布，不要把源 ref 填成另一个生产 tag。

## 当前功能

- 上线单申请：开发先指定本次发布业务线，再选择该业务线下需要上线的项目，并为项目指定分支、tag 或 commit；申请页支持上线单预览，也可以从历史上线单复制项目和来源后再编辑提交。
- 项目配置：新增、编辑、删除项目，维护 GitLab 地址、Project ID、默认分支、关联多条业务线。
- 业务线配置：新增、编辑、删除业务线，维护平台、tag 前缀和 tag 模板；删除已被项目使用的业务线时，需要选择替代业务线并迁移关联项目关系。
- 依赖顺序配置：管理员预先配置项目顺序和依赖关系，支持项目打包顺序上移/下移、新增、编辑、按依赖关系整行删除和清空依赖。
- 范围打 tag 构建：支持全量、后端、前端三种范围；对匹配项目按依赖顺序生成生产 tag，并等待当前项目自动构建结束后再处理下一个项目。
- Pipeline 流程：执行台按项目展示创建 Tag、Tag 触发的 Pipeline、GitLab jobs 状态；真实模式下创建 Tag 调用 GitLab Tags API，随后按 tag 查询 Pipeline，再通过 Jobs API 获取 build/deploy jobs；没有 deploy job 的 lib 类项目只展示构建链路。
- 断点续建：再次点击全量/后端/前端打 Tag 构建时，会跳过已构建成功项目，失败项目优先 retry 已有 build/package job，成功后继续后续项目。
- 来源调整：构建执行台可修改单项目本次发布来源分支、Tag 或 Commit；保存后会标记为来源已变更，下一次范围打 Tag 构建或单项目最新来源重打 Tag 会使用最新来源。
- 全量重打新 Tag 构建：清空当前上线单所有项目的 pipeline/job 状态，生成新的生产 tag，并从第一个项目重新按依赖顺序触发 GitLab CI。
- 单项目操作：支持重试原 Pipeline、最新来源重打 Tag 和部署；重试原 Pipeline 会对已有 build/package job 调用 GitLab retry job API，最新来源重打 Tag 会生成新 tag 并触发新的 pipeline。
- Job 日志：执行台支持直接查看构建/部署 job trace，并保留跳转 GitLab job 的链接；跳转链接优先使用项目配置里的 GitLab 仓库域名。
- 发布历史：持久化记录批次、项目、tag、pipeline、操作时间线，并支持清理不再需要的历史发布任务。
- 用户权限：管理员维护用户、角色、状态和重置密码；内置开发、发布经理、管理员三类角色。

Tag 模板支持 `{prefix}`、`{timestamp}`、`{datetime}`、`{date}`、`{releaseNo}`，其中时间戳按秒生成，格式为 `yyyyMMddHHmmss`。旧模板里的 `{date}` 会兼容为秒级时间戳。

当前 GitLab CI 流程以 tag 触发 pipeline 为准：平台创建 tag 后等待 GitLab 自动生成 pipeline，再查询该 pipeline 的 jobs。批量“打 Tag 构建”按钮不会传 `JOB_NAME`，而是调用 `/api/releases/:id/tag?target=all|backend|frontend&mode=resume`；如果某个项目失败会停止后续项目，再次点击同范围按钮会断点续建。若项目来源已修改，断点续建会为该项目生成新 tag，而不是 retry 旧 pipeline。单项目“重试原 Pipeline”用于原 pipeline 补发，“最新来源重打 Tag”用于基于新分支、Tag 或 Commit 触发新 pipeline。执行台会轮询当前上线单并同步 GitLab jobs，避免自动构建完成后页面仍停留在旧状态。

## 后续建议

- 接入完整 go-admin 菜单、Casbin 策略和操作日志表。
- 增加审批流节点：提交、审批、构建、部署、关闭。
- 增加 GitLab pipeline 状态轮询任务和失败日志抓取。
- 增加 LDAP/OIDC 登录，用企业账号替换内置账号。
