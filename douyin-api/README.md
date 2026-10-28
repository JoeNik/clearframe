# douyin-api 挂载文件说明

`docker-compose.yml` 会把本目录的两个文件只读挂载进旁挂容器
`evil0ctal/douyin_tiktok_download_api:V4.1.2`，覆盖镜像内的同名文件：

| 文件 | 容器内路径 | 作用 |
|---|---|---|
| `config.yaml` | `/app/crawlers/douyin/web/config.yaml` | 抖音抓取配置。**部署后如遇数据拉取失败，把浏览器里 douyin.com 的 Cookie 更新到这里的 `Cookie:` 行，然后 `docker compose restart douyin-api`** |
| `web_crawler.py` | `/app/crawlers/douyin/web/web_crawler.py` | 打了 **msToken 补丁**的爬虫：官方代码把请求参数里的 msToken 硬编码为空串，抖音自 2026 年起对作品列表接口强制校验它，实测必得 403。补丁在 a_bogus 签名前注入实时铸造的真实 msToken（搜 `ClearFrame patch` 可见改动点） |

注意：

- 这两个文件对应 **V4.1.2** 镜像。以后升级镜像版本时，先在容器里 diff 一遍同名文件，
  若官方已内置同类修复，可移除挂载改用镜像自带代码。
- 改完宿主机上的文件后需要 `docker compose restart douyin-api` 才生效（Python 启动时加载）。
