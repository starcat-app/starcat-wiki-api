# starcat-wiki-api

StarCat 外部文档站索引探测与跳转集成服务。

## 能力

1.  **索引探测**：探测 DeepWiki / Zread / Google Code Wiki 是否已索引某个 GitHub 仓库。
2.  **跳转链接**：返回探测结果对应的跳转 URL。
3.  **分级缓存**：根据探测结果（indexed/unknown/error 等）自动分级缓存结果。
4.  **SWR 缓存**：支持 Stale-While-Revalidate 模式，保证客户端极速响应。

## 运行

### 本地运行

1.  复制 `.env.example` 到 `.env` 并配置 `API_KEYS`。
2.  运行 `make run`。

### Docker 运行

```bash
make docker-build
make docker-run
```

## API

### GET /api/v1/wikis

探测单仓库。

**参数**：
- `owner`: 仓库所有者
- `repo`: 仓库名

### POST /api/v1/wikis/batch

批量探测。

## License

MIT
