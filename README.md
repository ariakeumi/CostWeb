# 资产看板

一个模仿“有数”逻辑的轻量 Web 应用，后端使用 Go + SQLite，前端使用原生 HTML/CSS/JS。

## 功能

- 资产录入：名称、分类、购入价格、购入日期、预期使用年限或目标日耗
- 资产编辑与删除：同一表单支持更新与移除资产
- 日均成本：按购入日至今天数实时计算
- 资产状态：服役中、已闲置、已售出，支持售出价格与售出日期
- 生命周期进度：按预期年限或目标日耗反推预计总天数并显示进度
- 总览统计：总资产、平均日耗、状态数量分布
- 分类统计：按分类聚合数量、总价值、平均日耗
- 图表页：分类价值条形图与月度购入趋势图

## 运行

```bash
go run .
```

默认访问地址：

```text
http://localhost:8080
```

## 测试

```bash
env GOCACHE=/tmp/go-build go test ./...
```

## Docker

容器默认以 `root` 用户运行。

单架构本地构建：

```bash
docker build -t costweb:local .
docker run --rm -p 8080:8080 costweb:local
```

多架构镜像构建并推送：

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t <your-registry>/costweb:latest \
  --push .
```

## GitHub Actions 自动推送 Docker Hub

已提供工作流：

[`docker-publish.yml`](/Users/umi/Documents/CostWeb/.github/workflows/docker-publish.yml)

默认行为：

- 推送到 `main` 分支时自动构建
- 推送 `v*` tag 时自动构建
- 构建 `linux/amd64` 和 `linux/arm64`
- 自动推送到 Docker Hub

需要在 GitHub 仓库 `Settings -> Secrets and variables -> Actions` 里添加：

- `DOCKERHUB_USERNAME`
- `DOCKERHUB_TOKEN`

其中 `DOCKERHUB_TOKEN` 建议使用 Docker Hub 的 Access Token，不要直接用密码。

如果你的默认分支不是 `main`，把工作流里的：

```yaml
branches:
  - main
```

改成你的实际分支名，例如 `master`。
