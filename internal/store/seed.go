package store

func (s *Store) seed() error {
	if err := s.seedCategories(); err != nil {
		return err
	}
	if err := s.seedAds(); err != nil {
		return err
	}
	if err := s.seedInterfaces(); err != nil {
		return err
	}
	// 本站自实现的能力不走上游同步，老库升级后单独补种（只补缺，不覆盖管理员已有的修改）
	return s.seedLocalInterfaces()
}

func (s *Store) seedCategories() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM categories`).Scan(&n); err != nil || n > 0 {
		return err
	}
	cats := []Category{
		{Slug: "parse", Name: "视频解析", Icon: "video", Sort: 10},
		{Slug: "data", Name: "数据分析", Icon: "gauge", Sort: 20},
		{Slug: "tool", Name: "实用工具", Icon: "tool", Sort: 30},
	}
	for _, c := range cats {
		if _, err := s.UpsertCategory(c); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seedAds() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM ad_slots`).Scan(&n); err != nil || n > 0 {
		return err
	}
	slots := []AdSlot{
		{Slot: "header", Name: "顶部横幅（全站）", Enabled: false,
			HTML: `<a class="ad-demo" href="#" target="_blank" rel="noopener"><span class="ad-tag">AD</span> 在这里投放你的广告位 · 支持任意 HTML / JS</a>`},
		{Slot: "hero", Name: "首页 Hero 下方", Enabled: false,
			HTML: `<a class="ad-demo" href="#" target="_blank" rel="noopener"><span class="ad-tag">AD</span> 首页黄金广告位 1200×90</a>`},
		{Slot: "list", Name: "接口列表内嵌（每 8 个卡片）", Enabled: false,
			HTML: `<a class="ad-demo" href="#" target="_blank" rel="noopener"><span class="ad-tag">AD</span> 信息流广告位</a>`},
		{Slot: "detail", Name: "接口详情页正文底部", Enabled: false,
			HTML: `<a class="ad-demo" href="#" target="_blank" rel="noopener"><span class="ad-tag">AD</span> 详情页广告位</a>`},
		{Slot: "sidebar", Name: "文档页右侧栏", Enabled: false,
			HTML: `<a class="ad-demo" href="#" target="_blank" rel="noopener"><span class="ad-tag">AD</span> 侧边栏广告 300×250</a>`},
		{Slot: "footer", Name: "页脚上方", Enabled: false,
			HTML: `<a class="ad-demo" href="#" target="_blank" rel="noopener"><span class="ad-tag">AD</span> 页脚广告位</a>`},
	}
	for _, a := range slots {
		if err := s.SaveAdSlot(a); err != nil {
			return err
		}
	}
	return nil
}

func p(name, typ string, required bool, def, desc string) Param {
	return Param{Name: name, Type: typ, Required: required, Default: def, Desc: desc}
}

// seedInterfaces 内置上游接口快照，用于离线首次启动；
// 联网后可在后台「从上游同步」刷新为最新清单。
func (s *Store) seedInterfaces() error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM interfaces`).Scan(&n); err != nil || n > 0 {
		return err
	}
	list := []Interface{
		{
			Slug: "dyjx", Name: "抖音无水印解析【原画】", Category: "parse",
			Method: "GET", Path: "/v1/dyjx", UpstreamPath: "/api/dyjx",
			Summary: "抖音去水印解析，支持解析图文、短视频和实况作品，支持短链接和长链接参数",
			Description: "抖音视频/图集/实况无水印原画级解析，返回统一JSON格式3.0媒体对象，支持video_backup备选与作品统计。\n\n统一JSON格式3.0：业务成功码code=200，收到HTTP200不代表业务成功，必须检查JSON中的code；失败时data为[]。媒体对象含type/title/desc/author/cover/url/quality/duration/size/size_label/create_time/publish_time/images/live_photo/music/extra等16基础字段；视频可含video_backup备选。标识按字符串处理避免精度丢失；数值运算用size；比率已是[0,1]小数。未使用可选参数应省略或传空串，勿传数组/对象/null。",
			Params: []Param{p("url", "string", true, "https://v.douyin.com/XBwlKFr1ya0/", "url 参数")},
			BodyExample: "url=https://v.douyin.com/XBwlKFr1ya0/",
			ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"解析成功\",\n  \"data\": {\n    \"type\": \"video\",\n    \"title\": \"示例作品标题\",\n    \"desc\": \"示例作品描述\",\n    \"author\": {\n      \"name\": \"示例作者\",\n      \"id\": \"author_001\",\n      \"avatar\": \"https://example.com/avatar.jpg\"\n    },\n    \"cover\": \"https://example.com/cover.jpg\",\n    \"url\": \"https://example.com/video.mp4\",\n    \"quality\": \"1080p\",\n    \"duration\": 15,\n    \"size\": 10485760,\n    \"size_label\": \"10.00MB\",\n    \"create_time\": 1735660800,\n    \"publish_time\": \"2025-01-01 00:00:00\",\n    \"images\": [],\n    \"live_photo\": [],\n    \"video_backup\": [\n      {\n        \"label\": \"720p\",\n        \"quality\": \"720p\",\n        \"url\": \"https://example.com/video_720p.mp4\",\n        \"bit_rate\": 1200000,\n        \"width\": 1280,\n        \"height\": 720,\n        \"format\": \"mp4\",\n        \"codec\": \"h264\",\n        \"quality_type\": 4,\n        \"gear_name\": \"流畅\",\n        \"size\": 5242880,\n        \"size_label\": \"5.00MB\"\n      }\n    ],\n    \"music\": {\n      \"title\": \"示例音乐\",\n      \"author\": \"示例音乐作者\",\n      \"url\": \"https://example.com/music.mp3\",\n      \"cover\": \"https://example.com/music_cover.jpg\"\n    },\n    \"extra\": {\n      \"aweme_id\": \"1234567890123456789\",\n      \"create_time\": 1735660800,\n      \"is_top\": 0,\n      \"statistics\": {\n        \"play_count\": 10000,\n        \"digg_count\": 100,\n        \"comment_count\": 10,\n        \"share_count\": 30,\n        \"collect_count\": 20,\n        \"interact_count\": 140,\n        \"interact_rate\": 0.014,\n        \"finish_rate\": 0.6\n      }\n    }\n  }\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 1000,
			Tags: []string{"去水印", "精选", "热门", "免费"}, Sort: 10,
		},
		{
			Slug: "svparse", Name: "短视频解析聚合", Category: "parse",
			Method: "GET", Path: "/v1/svparse", UpstreamPath: "/api/svparse",
			Summary: "短视频解析聚合支持33+平台，包含抖音、快手、小红书、bilibili、油管、TikTok、西瓜视频、好看视频、微视、梨",
			Description: "短视频解析聚合支持33+平台（抖音/快手/小红书/b站/油管/TikTok等），返回统一JSON格式3.0媒体对象。\n\n统一JSON格式3.0：业务成功码code=200，收到HTTP200不代表业务成功，必须检查JSON中的code；失败时data为[]。媒体对象含type/title/desc/author/cover/url/quality/duration/size/size_label/create_time/publish_time/images/live_photo/music/extra等16基础字段；视频可含video_backup备选。标识按字符串处理避免精度丢失；数值运算用size；比率已是[0,1]小数。未使用可选参数应省略或传空串，勿传数组/对象/null。",
			Params: []Param{p("url", "string", true, "https://v.douyin.com/XBwlKFr1ya0/", "url 参数")},
			BodyExample: "url=https://v.douyin.com/XBwlKFr1ya0/",
			ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"解析成功\",\n  \"data\": {\n    \"type\": \"video\",\n    \"title\": \"示例作品标题\",\n    \"desc\": \"示例作品描述\",\n    \"author\": {\n      \"name\": \"示例作者\",\n      \"id\": \"author_001\",\n      \"avatar\": \"https://example.com/avatar.jpg\"\n    },\n    \"cover\": \"https://example.com/cover.jpg\",\n    \"url\": \"https://example.com/video.mp4\",\n    \"quality\": \"1080p\",\n    \"duration\": 15,\n    \"size\": 10485760,\n    \"size_label\": \"10.00MB\",\n    \"create_time\": 1735660800,\n    \"publish_time\": \"2025-01-01 00:00:00\",\n    \"images\": [],\n    \"live_photo\": [],\n    \"video_backup\": [\n      {\n        \"label\": \"720p\",\n        \"quality\": \"720p\",\n        \"url\": \"https://example.com/video_720p.mp4\",\n        \"bit_rate\": 1200000,\n        \"width\": 1280,\n        \"height\": 720,\n        \"format\": \"mp4\",\n        \"codec\": \"h264\",\n        \"quality_type\": 4,\n        \"gear_name\": \"流畅\",\n        \"size\": 5242880,\n        \"size_label\": \"5.00MB\"\n      }\n    ],\n    \"music\": {\n      \"title\": \"示例音乐\",\n      \"author\": \"示例音乐作者\",\n      \"url\": \"https://example.com/music.mp3\",\n      \"cover\": \"https://example.com/music_cover.jpg\"\n    },\n    \"extra\": {\n      \"aweme_id\": \"1234567890123456789\",\n      \"create_time\": 1735660800,\n      \"is_top\": 0,\n      \"statistics\": {\n        \"play_count\": 10000,\n        \"digg_count\": 100,\n        \"comment_count\": 10,\n        \"share_count\": 30,\n        \"collect_count\": 20,\n        \"interact_count\": 140,\n        \"interact_rate\": 0.014,\n        \"finish_rate\": 0.6\n      }\n    }\n  }\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 213,
			Tags: []string{"去水印", "精选", "热门", "免费"}, Sort: 20,
		},
		{
			Slug: "wxsph", Name: "微信视频号解析", Category: "parse",
			Method: "GET", Path: "/v1/wxsph", UpstreamPath: "/api/wxsph",
			Summary: "微信视频号解析",
			Description: "微信视频号解析，返回统一JSON格式3.0媒体对象。\n\n统一JSON格式3.0：业务成功码code=200，收到HTTP200不代表业务成功，必须检查JSON中的code；失败时data为[]。媒体对象含type/title/desc/author/cover/url/quality/duration/size/size_label/create_time/publish_time/images/live_photo/music/extra等16基础字段；视频可含video_backup备选。标识按字符串处理避免精度丢失；数值运算用size；比率已是[0,1]小数。未使用可选参数应省略或传空串，勿传数组/对象/null。",
			Params: []Param{p("url", "string", true, "https://v.douyin.com/xxxxx/", "分享链接、短链接或作品ID字符串")},
			BodyExample: "url=https://v.douyin.com/xxxxx/",
			ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"解析成功\",\n  \"data\": {\n    \"type\": \"video\",\n    \"title\": \"示例作品标题\",\n    \"desc\": \"示例作品描述\",\n    \"author\": {\n      \"name\": \"示例作者\",\n      \"id\": \"author_001\",\n      \"avatar\": \"https://example.com/avatar.jpg\"\n    },\n    \"cover\": \"https://example.com/cover.jpg\",\n    \"url\": \"https://example.com/video.mp4\",\n    \"quality\": \"1080p\",\n    \"duration\": 15,\n    \"size\": 10485760,\n    \"size_label\": \"10.00MB\",\n    \"create_time\": 1735660800,\n    \"publish_time\": \"2025-01-01 00:00:00\",\n    \"images\": [],\n    \"live_photo\": [],\n    \"video_backup\": [\n      {\n        \"label\": \"720p\",\n        \"quality\": \"720p\",\n        \"url\": \"https://example.com/video_720p.mp4\",\n        \"bit_rate\": 1200000,\n        \"width\": 1280,\n        \"height\": 720,\n        \"format\": \"mp4\",\n        \"codec\": \"h264\",\n        \"quality_type\": 4,\n        \"gear_name\": \"流畅\",\n        \"size\": 5242880,\n        \"size_label\": \"5.00MB\"\n      }\n    ],\n    \"music\": {\n      \"title\": \"示例音乐\",\n      \"author\": \"示例音乐作者\",\n      \"url\": \"https://example.com/music.mp3\",\n      \"cover\": \"https://example.com/music_cover.jpg\"\n    },\n    \"extra\": {\n      \"aweme_id\": \"1234567890123456789\",\n      \"create_time\": 1735660800,\n      \"is_top\": 0,\n      \"statistics\": {\n        \"play_count\": 10000,\n        \"digg_count\": 100,\n        \"comment_count\": 10,\n        \"share_count\": 30,\n        \"collect_count\": 20,\n        \"interact_count\": 140,\n        \"interact_rate\": 0.014,\n        \"finish_rate\": 0.6\n      }\n    }\n  }\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 80,
			Tags: []string{"去水印", "精选", "热门", "免费"}, Sort: 30,
		},
		{
			Slug: "zjyh", Name: "某系列原画解析", Category: "parse",
			Method: "GET", Path: "/v1/zjyh", UpstreamPath: "/api/zjyh",
			Summary: "某系列获取原画质视频链接",
			Description: "某系列获取原画质视频，返回统一JSON格式3.0媒体对象。\n\n统一JSON格式3.0：业务成功码code=200，收到HTTP200不代表业务成功，必须检查JSON中的code；失败时data为[]。媒体对象含type/title/desc/author/cover/url/quality/duration/size/size_label/create_time/publish_time/images/live_photo/music/extra等16基础字段；视频可含video_backup备选。标识按字符串处理避免精度丢失；数值运算用size；比率已是[0,1]小数。未使用可选参数应省略或传空串，勿传数组/对象/null。",
			Params: []Param{p("video_id", "string", true, "v0200fg10000d55b84vog65n0ej27qvg", "video_id 参数")},
			BodyExample: "video_id=v0200fg10000d55b84vog65n0ej27qvg",
			ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"解析成功\",\n  \"data\": {\n    \"type\": \"video\",\n    \"title\": \"示例作品标题\",\n    \"desc\": \"示例作品描述\",\n    \"author\": {\n      \"name\": \"示例作者\",\n      \"id\": \"author_001\",\n      \"avatar\": \"https://example.com/avatar.jpg\"\n    },\n    \"cover\": \"https://example.com/cover.jpg\",\n    \"url\": \"https://example.com/video.mp4\",\n    \"quality\": \"1080p\",\n    \"duration\": 15,\n    \"size\": 10485760,\n    \"size_label\": \"10.00MB\",\n    \"create_time\": 1735660800,\n    \"publish_time\": \"2025-01-01 00:00:00\",\n    \"images\": [],\n    \"live_photo\": [],\n    \"video_backup\": [\n      {\n        \"label\": \"720p\",\n        \"quality\": \"720p\",\n        \"url\": \"https://example.com/video_720p.mp4\",\n        \"bit_rate\": 1200000,\n        \"width\": 1280,\n        \"height\": 720,\n        \"format\": \"mp4\",\n        \"codec\": \"h264\",\n        \"quality_type\": 4,\n        \"gear_name\": \"流畅\",\n        \"size\": 5242880,\n        \"size_label\": \"5.00MB\"\n      }\n    ],\n    \"music\": {\n      \"title\": \"示例音乐\",\n      \"author\": \"示例音乐作者\",\n      \"url\": \"https://example.com/music.mp3\",\n      \"cover\": \"https://example.com/music_cover.jpg\"\n    },\n    \"extra\": {\n      \"aweme_id\": \"1234567890123456789\",\n      \"create_time\": 1735660800,\n      \"is_top\": 0,\n      \"statistics\": {\n        \"play_count\": 10000,\n        \"digg_count\": 100,\n        \"comment_count\": 10,\n        \"share_count\": 30,\n        \"collect_count\": 20,\n        \"interact_count\": 140,\n        \"interact_rate\": 0.014,\n        \"finish_rate\": 0.6\n      }\n    }\n  }\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 29,
			Tags: []string{"去水印", "精选", "热门", "免费"}, Sort: 40,
		},
		{
			Slug: "jimeng", Name: "即梦去水印解析", Category: "parse",
			Method: "GET", Path: "/v1/jimeng", UpstreamPath: "/api/jimeng",
			Summary: "即梦ai视频去水印，支持隐藏和未发布的作品",
			Description: "即梦AI视频/图片去水印解析，支持隐藏和未发布作品，返回统一JSON格式3.0媒体对象。\n\n统一JSON格式3.0：业务成功码code=200，收到HTTP200不代表业务成功，必须检查JSON中的code；失败时data为[]。媒体对象含type/title/desc/author/cover/url/quality/duration/size/size_label/create_time/publish_time/images/live_photo/music/extra等16基础字段；视频可含video_backup备选。标识按字符串处理避免精度丢失；数值运算用size；比率已是[0,1]小数。未使用可选参数应省略或传空串，勿传数组/对象/null。",
			Params: []Param{p("url", "string", true, "https://jimeng.jianying.com/s/5I-I8pYkPFU/?t=8011", "url 参数")},
			BodyExample: "url=https://jimeng.jianying.com/s/5I-I8pYkPFU/?t=8011",
			ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"解析成功\",\n  \"data\": {\n    \"type\": \"video\",\n    \"title\": \"示例作品标题\",\n    \"desc\": \"示例作品描述\",\n    \"author\": {\n      \"name\": \"示例作者\",\n      \"id\": \"author_001\",\n      \"avatar\": \"https://example.com/avatar.jpg\"\n    },\n    \"cover\": \"https://example.com/cover.jpg\",\n    \"url\": \"https://example.com/video.mp4\",\n    \"quality\": \"1080p\",\n    \"duration\": 15,\n    \"size\": 10485760,\n    \"size_label\": \"10.00MB\",\n    \"create_time\": 1735660800,\n    \"publish_time\": \"2025-01-01 00:00:00\",\n    \"images\": [],\n    \"live_photo\": [],\n    \"video_backup\": [\n      {\n        \"label\": \"720p\",\n        \"quality\": \"720p\",\n        \"url\": \"https://example.com/video_720p.mp4\",\n        \"bit_rate\": 1200000,\n        \"width\": 1280,\n        \"height\": 720,\n        \"format\": \"mp4\",\n        \"codec\": \"h264\",\n        \"quality_type\": 4,\n        \"gear_name\": \"流畅\",\n        \"size\": 5242880,\n        \"size_label\": \"5.00MB\"\n      }\n    ],\n    \"music\": {\n      \"title\": \"示例音乐\",\n      \"author\": \"示例音乐作者\",\n      \"url\": \"https://example.com/music.mp3\",\n      \"cover\": \"https://example.com/music_cover.jpg\"\n    },\n    \"extra\": {\n      \"aweme_id\": \"1234567890123456789\",\n      \"create_time\": 1735660800,\n      \"is_top\": 0,\n      \"statistics\": {\n        \"play_count\": 10000,\n        \"digg_count\": 100,\n        \"comment_count\": 10,\n        \"share_count\": 30,\n        \"collect_count\": 20,\n        \"interact_count\": 140,\n        \"interact_rate\": 0.014,\n        \"finish_rate\": 0.6\n      }\n    }\n  }\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 7,
			Tags: []string{"去水印", "精选", "热门", "免费"}, Sort: 50,
		},
		{
			Slug: "doubao", Name: "豆包去水印解析", Category: "parse",
			Method: "GET", Path: "/v1/doubao", UpstreamPath: "/api/doubao",
			Summary: "豆包对话生图、视频去水印解析",
			Description: "豆包视频/图片去水印解析，返回统一JSON格式3.0媒体对象。\n\n统一JSON格式3.0：业务成功码code=200，收到HTTP200不代表业务成功，必须检查JSON中的code；失败时data为[]。媒体对象含type/title/desc/author/cover/url/quality/duration/size/size_label/create_time/publish_time/images/live_photo/music/extra等16基础字段；视频可含video_backup备选。标识按字符串处理避免精度丢失；数值运算用size；比率已是[0,1]小数。未使用可选参数应省略或传空串，勿传数组/对象/null。",
			Params: []Param{p("url", "string", true, "https://www.doubao.com/video-sharing?source_type=mobile&share_id=52137621563288578&video_id=v0269cg10004d81f0ealjht1q4b4sacg", "url 参数")},
			BodyExample: "url=https://www.doubao.com/video-sharing?source_type=mobile&share_id=52137621563288578&video_id=v0269cg10004d81f0ealjht1q4b4sacg",
			ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"解析成功\",\n  \"data\": {\n    \"type\": \"video\",\n    \"title\": \"示例作品标题\",\n    \"desc\": \"示例作品描述\",\n    \"author\": {\n      \"name\": \"示例作者\",\n      \"id\": \"author_001\",\n      \"avatar\": \"https://example.com/avatar.jpg\"\n    },\n    \"cover\": \"https://example.com/cover.jpg\",\n    \"url\": \"https://example.com/video.mp4\",\n    \"quality\": \"1080p\",\n    \"duration\": 15,\n    \"size\": 10485760,\n    \"size_label\": \"10.00MB\",\n    \"create_time\": 1735660800,\n    \"publish_time\": \"2025-01-01 00:00:00\",\n    \"images\": [],\n    \"live_photo\": [],\n    \"video_backup\": [\n      {\n        \"label\": \"720p\",\n        \"quality\": \"720p\",\n        \"url\": \"https://example.com/video_720p.mp4\",\n        \"bit_rate\": 1200000,\n        \"width\": 1280,\n        \"height\": 720,\n        \"format\": \"mp4\",\n        \"codec\": \"h264\",\n        \"quality_type\": 4,\n        \"gear_name\": \"流畅\",\n        \"size\": 5242880,\n        \"size_label\": \"5.00MB\"\n      }\n    ],\n    \"music\": {\n      \"title\": \"示例音乐\",\n      \"author\": \"示例音乐作者\",\n      \"url\": \"https://example.com/music.mp3\",\n      \"cover\": \"https://example.com/music_cover.jpg\"\n    },\n    \"extra\": {\n      \"aweme_id\": \"1234567890123456789\",\n      \"create_time\": 1735660800,\n      \"is_top\": 0,\n      \"statistics\": {\n        \"play_count\": 10000,\n        \"digg_count\": 100,\n        \"comment_count\": 10,\n        \"share_count\": 30,\n        \"collect_count\": 20,\n        \"interact_count\": 140,\n        \"interact_rate\": 0.014,\n        \"finish_rate\": 0.6\n      }\n    }\n  }\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 2,
			Tags: []string{"去水印", "精选", "热门", "免费"}, Sort: 60,
		},
		{
			Slug: "ksjx", Name: "快手解析", Category: "parse",
			Method: "GET", Path: "/v1/ksjx", UpstreamPath: "/api/ksjx",
			Summary: "快手视频、图集、实况去水印解析",
			Description: "快手视频/图集/实况去水印解析，返回统一JSON格式3.0媒体对象。\n\n统一JSON格式3.0：业务成功码code=200，收到HTTP200不代表业务成功，必须检查JSON中的code；失败时data为[]。媒体对象含type/title/desc/author/cover/url/quality/duration/size/size_label/create_time/publish_time/images/live_photo/music/extra等16基础字段；视频可含video_backup备选。标识按字符串处理避免精度丢失；数值运算用size；比率已是[0,1]小数。未使用可选参数应省略或传空串，勿传数组/对象/null。",
			Params: []Param{p("url", "string", true, "https://v.kuaishou.com/KAjGG5zb", "url 参数")},
			BodyExample: "url=https://v.kuaishou.com/KAjGG5zb",
			ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"解析成功\",\n  \"data\": {\n    \"type\": \"video\",\n    \"title\": \"示例作品标题\",\n    \"desc\": \"示例作品描述\",\n    \"author\": {\n      \"name\": \"示例作者\",\n      \"id\": \"author_001\",\n      \"avatar\": \"https://example.com/avatar.jpg\"\n    },\n    \"cover\": \"https://example.com/cover.jpg\",\n    \"url\": \"https://example.com/video.mp4\",\n    \"quality\": \"1080p\",\n    \"duration\": 15,\n    \"size\": 10485760,\n    \"size_label\": \"10.00MB\",\n    \"create_time\": 1735660800,\n    \"publish_time\": \"2025-01-01 00:00:00\",\n    \"images\": [],\n    \"live_photo\": [],\n    \"video_backup\": [\n      {\n        \"label\": \"720p\",\n        \"quality\": \"720p\",\n        \"url\": \"https://example.com/video_720p.mp4\",\n        \"bit_rate\": 1200000,\n        \"width\": 1280,\n        \"height\": 720,\n        \"format\": \"mp4\",\n        \"codec\": \"h264\",\n        \"quality_type\": 4,\n        \"gear_name\": \"流畅\",\n        \"size\": 5242880,\n        \"size_label\": \"5.00MB\"\n      }\n    ],\n    \"music\": {\n      \"title\": \"示例音乐\",\n      \"author\": \"示例音乐作者\",\n      \"url\": \"https://example.com/music.mp3\",\n      \"cover\": \"https://example.com/music_cover.jpg\"\n    },\n    \"extra\": {\n      \"aweme_id\": \"1234567890123456789\",\n      \"create_time\": 1735660800,\n      \"is_top\": 0,\n      \"statistics\": {\n        \"play_count\": 10000,\n        \"digg_count\": 100,\n        \"comment_count\": 10,\n        \"share_count\": 30,\n        \"collect_count\": 20,\n        \"interact_count\": 140,\n        \"interact_rate\": 0.014,\n        \"finish_rate\": 0.6\n      }\n    }\n  }\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 2,
			Tags: []string{"去水印", "精选", "热门", "免费"}, Sort: 70,
		},
		{
			Slug: "bilibili", Name: "bilibili解析", Category: "parse",
			Method: "GET", Path: "/v1/bilibili", UpstreamPath: "/api/bilibili",
			Summary: "只能解析bilibili短视频,画质为1080，支持合集",
			Description: "bilibili短视频解析(1080p)，支持合集，返回统一JSON格式3.0媒体对象。\n\n统一JSON格式3.0：业务成功码code=200，收到HTTP200不代表业务成功，必须检查JSON中的code；失败时data为[]。媒体对象含type/title/desc/author/cover/url/quality/duration/size/size_label/create_time/publish_time/images/live_photo/music/extra等16基础字段；视频可含video_backup备选。标识按字符串处理避免精度丢失；数值运算用size；比率已是[0,1]小数。未使用可选参数应省略或传空串，勿传数组/对象/null。",
			Params: []Param{p("url", "string", true, "https://b23.tv/XhtfoyZ", "url 参数")},
			BodyExample: "url=https://b23.tv/XhtfoyZ",
			ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"解析成功\",\n  \"data\": {\n    \"type\": \"video\",\n    \"title\": \"示例作品标题\",\n    \"desc\": \"示例作品描述\",\n    \"author\": {\n      \"name\": \"示例作者\",\n      \"id\": \"author_001\",\n      \"avatar\": \"https://example.com/avatar.jpg\"\n    },\n    \"cover\": \"https://example.com/cover.jpg\",\n    \"url\": \"https://example.com/video.mp4\",\n    \"quality\": \"1080p\",\n    \"duration\": 15,\n    \"size\": 10485760,\n    \"size_label\": \"10.00MB\",\n    \"create_time\": 1735660800,\n    \"publish_time\": \"2025-01-01 00:00:00\",\n    \"images\": [],\n    \"live_photo\": [],\n    \"video_backup\": [\n      {\n        \"label\": \"720p\",\n        \"quality\": \"720p\",\n        \"url\": \"https://example.com/video_720p.mp4\",\n        \"bit_rate\": 1200000,\n        \"width\": 1280,\n        \"height\": 720,\n        \"format\": \"mp4\",\n        \"codec\": \"h264\",\n        \"quality_type\": 4,\n        \"gear_name\": \"流畅\",\n        \"size\": 5242880,\n        \"size_label\": \"5.00MB\"\n      }\n    ],\n    \"music\": {\n      \"title\": \"示例音乐\",\n      \"author\": \"示例音乐作者\",\n      \"url\": \"https://example.com/music.mp3\",\n      \"cover\": \"https://example.com/music_cover.jpg\"\n    },\n    \"extra\": {\n      \"aweme_id\": \"1234567890123456789\",\n      \"create_time\": 1735660800,\n      \"is_top\": 0,\n      \"statistics\": {\n        \"play_count\": 10000,\n        \"digg_count\": 100,\n        \"comment_count\": 10,\n        \"share_count\": 30,\n        \"collect_count\": 20,\n        \"interact_count\": 140,\n        \"interact_rate\": 0.014,\n        \"finish_rate\": 0.6\n      }\n    }\n  }\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 1,
			Tags: []string{"去水印", "精选", "热门", "免费"}, Sort: 80,
		},
		{
			Slug: "dydj", Name: "dy特别短的剧", Category: "parse",
			Method: "GET", Path: "/v1/dydj", UpstreamPath: "/api/dydj",
			Summary: "支持获取视频视频原画，播放量等重要信息，比之前的标签数据多",
			Description: "抖音短剧视频解析，支持获取视频原画及播放量等重要信息，返回统一JSON格式3.0媒体对象。\n\n统一JSON格式3.0：业务成功码code=200，收到HTTP200不代表业务成功，必须检查JSON中的code；失败时data为[]。媒体对象含type/title/desc/author/cover/url/quality/duration/size/size_label/create_time/publish_time/images/live_photo/music/extra等16基础字段；视频可含video_backup备选。标识按字符串处理避免精度丢失；数值运算用size；比率已是[0,1]小数。未使用可选参数应省略或传空串，勿传数组/对象/null。",
			Params: []Param{p("url", "string", true, "https://v.douyin.com/TL8CAt2bOYE/", "url 参数")},
			BodyExample: "url=https://v.douyin.com/TL8CAt2bOYE/",
			ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"解析成功\",\n  \"data\": {\n    \"type\": \"video\",\n    \"title\": \"示例作品标题\",\n    \"desc\": \"示例作品描述\",\n    \"author\": {\n      \"name\": \"示例作者\",\n      \"id\": \"author_001\",\n      \"avatar\": \"https://example.com/avatar.jpg\"\n    },\n    \"cover\": \"https://example.com/cover.jpg\",\n    \"url\": \"https://example.com/video.mp4\",\n    \"quality\": \"1080p\",\n    \"duration\": 15,\n    \"size\": 10485760,\n    \"size_label\": \"10.00MB\",\n    \"create_time\": 1735660800,\n    \"publish_time\": \"2025-01-01 00:00:00\",\n    \"images\": [],\n    \"live_photo\": [],\n    \"video_backup\": [\n      {\n        \"label\": \"720p\",\n        \"quality\": \"720p\",\n        \"url\": \"https://example.com/video_720p.mp4\",\n        \"bit_rate\": 1200000,\n        \"width\": 1280,\n        \"height\": 720,\n        \"format\": \"mp4\",\n        \"codec\": \"h264\",\n        \"quality_type\": 4,\n        \"gear_name\": \"流畅\",\n        \"size\": 5242880,\n        \"size_label\": \"5.00MB\"\n      }\n    ],\n    \"music\": {\n      \"title\": \"示例音乐\",\n      \"author\": \"示例音乐作者\",\n      \"url\": \"https://example.com/music.mp3\",\n      \"cover\": \"https://example.com/music_cover.jpg\"\n    },\n    \"extra\": {\n      \"aweme_id\": \"1234567890123456789\",\n      \"create_time\": 1735660800,\n      \"is_top\": 0,\n      \"statistics\": {\n        \"play_count\": 10000,\n        \"digg_count\": 100,\n        \"comment_count\": 10,\n        \"share_count\": 30,\n        \"collect_count\": 20,\n        \"interact_count\": 140,\n        \"interact_rate\": 0.014,\n        \"finish_rate\": 0.6\n      }\n    }\n  }\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 0,
			Tags: []string{"去水印", "精选", "热门", "免费"}, Sort: 90,
		},
		{
			Slug: "dytj", Name: "抖音作品统计", Category: "data",
			Method: "GET", Path: "/v1/dytj", UpstreamPath: "/api/dytj",
			Summary: "抖音作品统计",
			Description: "抖音作品统计详情接口，返回近30条内当前作品的详细数据\n\n抖音作品统计接口（统一JSON格式3.0第6节）：业务成功码code=200。成功data仅有extra.statistics累计统计与statistics_trend逐日趋势，不含媒体字段与data.type。统计字段按可用性返回，缺失不等于0，明确0为有效值；标识按字符串处理避免精度丢失。成功至少需一个有效累计统计字段或一条有效日趋势。仅趋势可用时statistics可为{}。累计计数非负；日增量可为负勿取绝对值。比率已是[0,1]小数。数据无有效统计时返回业务错误，data为[]。",
			Params: []Param{p("url", "string", false, "https://www.douyin.com/video/7699804945230384411", "作品链接、短链接、分享文案或作品ID字符串；与id至少提供一项"), p("id", "string", false, "7699804945230384411", "作品ID（15-22位、非0开头的十进制字符串）；与url至少提供一项，显式ID优先于url"), p("aweme_id", "string", false, "7699804945230384411", "同一作品ID的兼容参数名，可替代id；多个ID参数必须一致"), p("item_id", "string", false, "7699804945230384411", "同一作品ID的兼容参数名，可替代id"), p("key", "string", false, "YOUR_ACCESS_KEY", "访问密钥，仅用于URL查询参数；建议使用X-API-Key请求头，JSON请求体中的key不作为访问凭证")},
			BodyExample: "",
			ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"查询成功-iga\",\n  \"data\": {\n    \"extra\": {\n      \"aweme_id\": \"7672341550441123122\",\n      \"statistics\": {\n        \"play_count\": 178484,\n        \"digg_count\": 2099,\n        \"comment_count\": 32,\n        \"collect_count\": 259,\n        \"share_count\": 81,\n        \"interact_count\": 2212,\n        \"interact_rate\": 0.0123\n      },\n      \"author_extra\": {\n        \"follower_count\": 104385\n      },\n      \"statistics_trend\": {\n        \"granularity\": \"day\",\n        \"start_date\": \"2026-08-10\",\n        \"end_date\": \"2026-09-08\",\n        \"days\": 30,\n        \"daily\": [\n          {\n            \"date\": \"2026-08-10\",\n            \"play_count\": 12851,\n            \"valid_play_count\": 11468,\n            \"digg_count\": 220,\n            \"comment_count\": 3,\n            \"share_count\": 7,\n            \"collect_count\": 23,\n            \"interact_count\": 230,\n            \"interact_rate\": 0.0178,\n            \"finish_rate\": 0.0237,\n            \"hot_score\": 22571\n          },\n          {\n            \"date\": \"2026-08-11\",\n            \"play_count\": 16576,\n            \"valid_play_count\": 9325,\n            \"digg_count\": 154,\n            \"comment_count\": 5,\n            \"share_count\": 1,\n            \"collect_count\": 19,\n            \"interact_count\": 160,\n            \"interact_rate\": 0.0096,\n            \"finish_rate\": 0.0742,\n            \"hot_score\": 23657\n          },\n          {\n            \"date\": \"2026-08-12\",\n            \"play_count\": 8928,\n            \"valid_play_count\": 5004,\n            \"digg_count\": 92,\n            \"comment_count\": 0,\n            \"share_count\": 0,\n            \"collect_count\": 10,\n            \"interact_count\": 92,\n            \"interact_rate\": 0.0103,\n            \"finish_rate\": 0.0745,\n            \"hot_score\": 13114\n          },\n          {\n            \"date\": \"2026-08-13\",\n            \"play_count\": 4558,\n            \"valid_play_count\": 3114,\n            \"digg_count\": 45,\n            \"comment_count\": 1,\n            \"share_count\": 2,\n            \"collect_count\": 4,\n            \"interact_count\": 48,\n            \"interact_rate\": 0.0105,\n            \"finish_rate\": 0.095,\n            \"hot_score\": 7908\n          },\n          {\n            \"date\": \"2026-08-14\",\n            \"play_count\": 3542,\n            \"valid_play_count\": 2151,\n            \"digg_count\": 38,\n            \"comment_count\": 0,\n            \"share_count\": 1,\n            \"collect_count\": 6,\n            \"interact_count\": 39,\n            \"interact_rate\": 0.011,\n            \"finish_rate\": 0.1417,\n            \"hot_score\": 6579\n          },\n          {\n            \"date\": \"2026-08-15\",\n            \"play_count\": 3544,\n            \"valid_play_count\": 2398,\n            \"digg_count\": 28,\n            \"comment_count\": 0,\n            \"share_count\": 3,\n            \"collect_count\": 8,\n            \"interact_count\": 31,\n            \"interact_rate\": 0.0087,\n            \"finish_rate\": 0.1597,\n            \"hot_score\": 7006\n          },\n          {\n            \"date\": \"2026-08-16\",\n            \"play_count\": 4882,\n            \"valid_play_count\": 3403,\n            \"digg_count\": 71,\n            \"comment_count\": 0,\n            \"share_count\": 4,\n            \"collect_count\": 9,\n            \"interact_count\": 75,\n            \"interact_rate\": 0.0153,\n            \"finish_rate\": 0.1298,\n            \"hot_score\": 8917\n          },\n          {\n            \"date\": \"2026-08-17\",\n            \"play_count\": 5842,\n            \"valid_play_count\": 3939,\n            \"digg_count\": 70,\n            \"comment_count\": 3,\n            \"share_count\": 7,\n            \"collect_count\": 17,\n            \"interact_count\": 80,\n            \"interact_rate\": 0.0136,\n            \"finish_rate\": 0.1231,\n            \"hot_score\": 10201\n          },\n          {\n            \"date\": \"2026-08-18\",\n            \"play_count\": 3927,\n            \"valid_play_count\": 2554,\n            \"digg_count\": 52,\n            \"comment_count\": -2,\n            \"share_count\": 0,\n            \"collect_count\": 1,\n            \"interact_count\": 50,\n            \"interact_\n…",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 0,
			Tags: []string{"去水印", "免费"}, Sort: 100,
		},
		{
			Slug: "bbhj", Name: "宝贝回家", Category: "tool",
			Method: "GET", Path: "/v1/bbhj", UpstreamPath: "/api/bbhj",
			Summary: "宝贝回家  关爱儿童我们共同的责任",
			Description: "宝贝回家  关爱儿童我们共同的责任，公益接口",
			Params: []Param{},
			BodyExample: "",
			ResponseExample: "{\n  \"source_url\": \"https://so.baobeihuijia.com/bbhj/contents/4/*.html\",\n  \"category_code\": \"4\",\n  \"category_name\": \"宝贝寻家\",\n  \"detail_id\": \"*\",\n  \"record_no\": \"*\",\n  \"name\": \"*\",\n  \"sex\": \"*\",\n  \"birth_date\": \"*\",\n  \"lost_height\": \"未知\",\n  \"lost_time\": \"*\",\n  \"lost_location\": \"*\",\n  \"current_location\": \"*\",\n  \"feature\": \"*\",\n  \"other_details\": \"*\",\n  \"registered_at\": \"*\",\n  \"volunteer\": \"*\",\n  \"lost_type\": \"\",\n  \"photo_url\": \"\",\n  \"has_photo\": false,\n  \"raw_fields\": {\n    \"寻亲类别\": \"宝贝寻家\",\n    \"失踪类型\": \"\",\n    \"寻亲编号\": \"*\",\n    \"姓名\": \"*\",\n    \"性别\": \"*\",\n    \"出生日期\": \"*\",\n    \"失踪时身高\": \"未知CM\",\n    \"失踪时间\": \"*\",\n    \"失踪地点\": \"*\",\n    \"失踪人所在地\": \"*\",\n    \"失踪者特征描述\": \"*\",\n    \"其他资料\": \"*\",\n    \"注册时间\": \"*\",\n    \"跟进志愿者\": \"\"\n  },\n  \"list_page\": *,\n  \"list_page_url\": \"https://so.baobeihuijia.com/bbhj/channels/*.html\",\n  \"list_registered_date\": \"*\",\n  \"list_image_url\": \"\"\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 0,
			Tags: []string{"去水印", "精选", "热门", "免费"}, Sort: 110,
		},
		{
			Slug: "dola", Name: "dola去水印解析", Category: "parse",
			Method: "GET", Path: "/v1/dola", UpstreamPath: "/api/dola",
			Summary: "dola去水印解析",
			Description: "dola视频去水印解析，返回统一JSON格式3.0媒体对象。\n\n统一JSON格式3.0：业务成功码code=200，收到HTTP200不代表业务成功，必须检查JSON中的code；失败时data为[]。媒体对象含type/title/desc/author/cover/url/quality/duration/size/size_label/create_time/publish_time/images/live_photo/music/extra等16基础字段；视频可含video_backup备选。标识按字符串处理避免精度丢失；数值运算用size；比率已是[0,1]小数。未使用可选参数应省略或传空串，勿传数组/对象/null。",
			Params: []Param{p("url", "string", true, "https://www.dola.com/thread/xQwpNNt8dYkgWIudL", "url 参数")},
			BodyExample: "url=https://www.dola.com/thread/xQwpNNt8dYkgWIudL",
			ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"解析成功\",\n  \"data\": {\n    \"type\": \"video\",\n    \"title\": \"示例作品标题\",\n    \"desc\": \"示例作品描述\",\n    \"author\": {\n      \"name\": \"示例作者\",\n      \"id\": \"author_001\",\n      \"avatar\": \"https://example.com/avatar.jpg\"\n    },\n    \"cover\": \"https://example.com/cover.jpg\",\n    \"url\": \"https://example.com/video.mp4\",\n    \"quality\": \"1080p\",\n    \"duration\": 15,\n    \"size\": 10485760,\n    \"size_label\": \"10.00MB\",\n    \"create_time\": 1735660800,\n    \"publish_time\": \"2025-01-01 00:00:00\",\n    \"images\": [],\n    \"live_photo\": [],\n    \"video_backup\": [\n      {\n        \"label\": \"720p\",\n        \"quality\": \"720p\",\n        \"url\": \"https://example.com/video_720p.mp4\",\n        \"bit_rate\": 1200000,\n        \"width\": 1280,\n        \"height\": 720,\n        \"format\": \"mp4\",\n        \"codec\": \"h264\",\n        \"quality_type\": 4,\n        \"gear_name\": \"流畅\",\n        \"size\": 5242880,\n        \"size_label\": \"5.00MB\"\n      }\n    ],\n    \"music\": {\n      \"title\": \"示例音乐\",\n      \"author\": \"示例音乐作者\",\n      \"url\": \"https://example.com/music.mp3\",\n      \"cover\": \"https://example.com/music_cover.jpg\"\n    },\n    \"extra\": {\n      \"aweme_id\": \"1234567890123456789\",\n      \"create_time\": 1735660800,\n      \"is_top\": 0,\n      \"statistics\": {\n        \"play_count\": 10000,\n        \"digg_count\": 100,\n        \"comment_count\": 10,\n        \"share_count\": 30,\n        \"collect_count\": 20,\n        \"interact_count\": 140,\n        \"interact_rate\": 0.014,\n        \"finish_rate\": 0.6\n      }\n    }\n  }\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 0,
			Tags: []string{"去水印", "精选", "热门", "免费"}, Sort: 120,
		},
		{
			Slug: "pipigx", Name: "皮皮搞笑去水印解析", Category: "parse",
			Method: "GET", Path: "/v1/pipigx", UpstreamPath: "/api/pipigx",
			Summary: "皮皮搞笑无水印解析！",
			Description: "皮皮搞笑视频无水印解析，返回统一JSON格式3.0媒体对象。\n\n统一JSON格式3.0：业务成功码code=200，收到HTTP200不代表业务成功，必须检查JSON中的code；失败时data为[]。媒体对象含type/title/desc/author/cover/url/quality/duration/size/size_label/create_time/publish_time/images/live_photo/music/extra等16基础字段；视频可含video_backup备选。标识按字符串处理避免精度丢失；数值运算用size；比率已是[0,1]小数。未使用可选参数应省略或传空串，勿传数组/对象/null。",
			Params: []Param{p("url", "string", true, "https://h5.pipigx.com/pp/post/845304437502?zy_to=copy_link&share_count=1&m=68d879cb21cc61778b936b9458e65983&app=&type=post&did=b664d80b070ed4c8&mid=8519906662520&pid=845304437502", "url 参数")},
			BodyExample: "url=https://h5.pipigx.com/pp/post/845304437502?zy_to=copy_link&share_count=1&m=68d879cb21cc61778b936b9458e65983&app=&type=post&did=b664d80b070ed4c8&mid=8519906662520&pid=845304437502",
			ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"解析成功\",\n  \"data\": {\n    \"type\": \"video\",\n    \"title\": \"示例作品标题\",\n    \"desc\": \"示例作品描述\",\n    \"author\": {\n      \"name\": \"示例作者\",\n      \"id\": \"author_001\",\n      \"avatar\": \"https://example.com/avatar.jpg\"\n    },\n    \"cover\": \"https://example.com/cover.jpg\",\n    \"url\": \"https://example.com/video.mp4\",\n    \"quality\": \"1080p\",\n    \"duration\": 15,\n    \"size\": 10485760,\n    \"size_label\": \"10.00MB\",\n    \"create_time\": 1735660800,\n    \"publish_time\": \"2025-01-01 00:00:00\",\n    \"images\": [],\n    \"live_photo\": [],\n    \"video_backup\": [\n      {\n        \"label\": \"720p\",\n        \"quality\": \"720p\",\n        \"url\": \"https://example.com/video_720p.mp4\",\n        \"bit_rate\": 1200000,\n        \"width\": 1280,\n        \"height\": 720,\n        \"format\": \"mp4\",\n        \"codec\": \"h264\",\n        \"quality_type\": 4,\n        \"gear_name\": \"流畅\",\n        \"size\": 5242880,\n        \"size_label\": \"5.00MB\"\n      }\n    ],\n    \"music\": {\n      \"title\": \"示例音乐\",\n      \"author\": \"示例音乐作者\",\n      \"url\": \"https://example.com/music.mp3\",\n      \"cover\": \"https://example.com/music_cover.jpg\"\n    },\n    \"extra\": {\n      \"aweme_id\": \"1234567890123456789\",\n      \"create_time\": 1735660800,\n      \"is_top\": 0,\n      \"statistics\": {\n        \"play_count\": 10000,\n        \"digg_count\": 100,\n        \"comment_count\": 10,\n        \"share_count\": 30,\n        \"collect_count\": 20,\n        \"interact_count\": 140,\n        \"interact_rate\": 0.014,\n        \"finish_rate\": 0.6\n      }\n    }\n  }\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 0,
			Tags: []string{"去水印", "免费"}, Sort: 130,
		},
		{
			Slug: "domailcx", Name: "域名tools", Category: "tool",
			Method: "GET", Path: "/v1/domailcx", UpstreamPath: "/api/domailcx",
			Summary: "各后缀域名价格查询",
			Description: "域名价格查询工具，支持4种查询模式：按后缀查全部注册商报价、按注册商查全部后缀报价、单点询价、优惠查询；返回注册/续费/转入价格并自动换算人民币。\n\n参数组合：domain+order=按后缀查全部注册商；registrar+order=按注册商查全部后缀；registrar+domain=单点询价；registrar+domain+order=查优惠。价格优先使用*_cny人民币字段；n/a代表该注册商不适用该业务；业务码100=成功，非100读取msg。",
			Params: []Param{p("domain", "string", false, "com", "域名后缀，大小写不敏感；预设常用后缀：com / net / org / io / cn / top / xyz / online / shop / vip / cc / me 等，任意后缀均可查询"), p("registrar", "string", false, "aliyun", "注册商代码，预设常用：aliyun(阿里云) / tencent(腾讯云) / huawei(华为云) / godaddy / namecheap / cloudflare 等；完整代码见注册商列表接口"), p("order", "string", false, "register", "业务类型（预设可选值）：register=注册 / renew=续费 / transfer=转入；同时作为列表排序维度"), p("limit", "int", false, "20", "分页单页条数，预设可选：10 / 20 / 50 / 100；不传则返回全部数据"), p("page", "int", false, "1", "分页页码，从1开始，配合 limit 翻页使用")},
			BodyExample: "",
			ResponseExample: "{\n  \"code\": 100,\n  \"data\": {\n    \"page\": 1,\n    \"count\": 20,\n    \"total\": 120,\n    \"price\": [\n      {\n        \"registrar\": \"aliyun\",\n        \"registrarname\": \"阿里云\",\n        \"registrarweb\": \"https://wanwang.aliyun.com\",\n        \"register\": \"65.00\",\n        \"renew\": \"69.00\",\n        \"transfer\": \"55.00\",\n        \"register_cny\": 65,\n        \"renew_cny\": 69,\n        \"transfer_cny\": 55,\n        \"currency\": \"cny\",\n        \"currencyname\": \"元\",\n        \"currencytype\": \"人民币\",\n        \"promocode\": {\n          \"register\": true,\n          \"renew\": false,\n          \"transfer\": false\n        },\n        \"updatedtime\": \"2026-09-01 12:00:00\"\n      }\n    ]\n  }\n}",
			PricingMode: "free", Price: 0.0, PriceUnit: "次",
			RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 0,
			Tags: []string{"去水印", "精选", "热门", "免费"}, Sort: 140,
		},
	}
	list = append(list, dyzzInterface())
	for _, it := range list {
		if _, err := s.SaveInterface(it); err != nil {
			return err
		}
	}
	return nil
}

// dyzzInterface 本站自实现的本地能力：「抖音作者作品分析」。
// 不依赖上游接口市场，走旁挂的抖音数据服务，升级老库时由 seedLocalInterfaces 补种。
func dyzzInterface() Interface {
	return Interface{
		Slug: "dyzz", Name: "抖音作者作品分析", Category: "data",
		Method: "GET", Path: "/v1/dyzz", UpstreamPath: "/v1/dyzz",
		Summary: "粘贴作者主页链接，自动拉取全部作品并逐条统计点赞/评论/分享/收藏",
		Description: "输入作者主页链接（App 内分享主页复制）或分享文案，本站自动解析作者标识并翻页拉取其公开作品列表，每条作品带点赞、评论、分享、收藏数据与作品链接，并输出汇总指标，免去逐个复制作品链接。\n\n由本站直连旁挂的抖音数据服务（Douyin_TikTok_Download_API）实现，不经上游转发。limit 为最多拉取条数（默认 50，上限 200）。\n\n数据口径：抖音 web 通道不提供真实播放量，本能力只统计点赞/评论/分享/收藏；单作品的播放量与 30 天逐日趋势请用「抖音作品统计」对单个作品查询。若数据服务临时被平台风控，稍后重试即可。",
		Params: []Param{
			p("url", "string", true, "", "作者主页链接、主页分享文案或 sec_uid；注意不是单条作品链接"),
			p("limit", "int", false, "50", "最多拉取的作品条数，默认 50，上限 200"),
		},
		BodyExample: "url=https://v.douyin.com/xxxxxx/&limit=50",
		ResponseExample: "{\n  \"code\": 200,\n  \"msg\": \"查询成功\",\n  \"data\": {\n    \"sec_uid\": \"MS4wLjABAAAA...\",\n    \"author\": {\n      \"sec_uid\": \"MS4wLjABAAAA...\",\n      \"nickname\": \"示例作者\",\n      \"avatar\": \"https://...\",\n      \"signature\": \"...\",\n      \"follower_count\": 104385,\n      \"aweme_count\": 320,\n      \"total_favorited\": 1234567\n    },\n    \"videos\": [\n      {\n        \"aweme_id\": \"7587074832588655881\",\n        \"title\": \"示例作品标题\",\n        \"desc\": \"示例作品描述\",\n        \"kind\": \"video\",\n        \"url\": \"https://www.douyin.com/video/7587074832588655881\",\n        \"cover\": \"https://...\",\n        \"create_time\": 1769089903,\n        \"duration\": 15,\n        \"stats\": {\n          \"digg_count\": 2099,\n          \"comment_count\": 32,\n          \"share_count\": 81,\n          \"collect_count\": 259\n        }\n      }\n    ],\n    \"summary\": {\n      \"video_count\": 50,\n      \"total_digg\": 58231,\n      \"total_comment\": 1204,\n      \"total_share\": 903,\n      \"total_collect\": 4102,\n      \"avg_digg\": 1164,\n      \"latest_time\": 1769089903,\n      \"earliest_time\": 1735660800,\n      \"top_aweme_id\": \"7587074832588655881\",\n      \"top_title\": \"示例作品标题\",\n      \"top_digg\": 2099,\n      \"pages\": 3,\n      \"truncated\": false\n    },\n    \"source\": \"v4\"\n  }\n}",
		PricingMode: "free", Price: 0.0, PriceUnit: "次",
		RequiresLogin: false, Status: "online", LatencyMs: 0, Popularity: 0,
		Tags: []string{"数据分析", "批量"}, Sort: 105,
	}
}

// seedLocalInterfaces 老库升级补种：只补缺失的本地能力，不覆盖已有配置
func (s *Store) seedLocalInterfaces() error {
	local := []Interface{dyzzInterface()}
	for _, it := range local {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(1) FROM interfaces WHERE slug = ?`, it.Slug).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		if _, err := s.SaveInterface(it); err != nil {
			return err
		}
	}
	return nil
}
