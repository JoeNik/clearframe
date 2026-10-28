package store

import "time"

// Param 描述一个请求参数
type Param struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
	Default  string `json:"default"`
	Desc     string `json:"desc"`
}

// Category 接口分类
type Category struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Icon string `json:"icon"`
	Sort int    `json:"sort"`
}

// Interface 对外提供的接口
type Interface struct {
	ID                int64     `json:"id"`
	Slug              string    `json:"slug"`
	Name              string    `json:"name"`
	Category          string    `json:"category"`
	Method            string    `json:"method"`
	Path              string    `json:"path"`
	UpstreamPath      string    `json:"upstreamPath"`   // 转发到上游的路径，留空则与 Path 相同
	UpstreamMethod    string    `json:"upstreamMethod"` // 转发到上游的方法，留空则与 Method 相同
	Summary           string    `json:"summary"`
	Description       string    `json:"description"`
	Params            []Param   `json:"params"`
	BodyExample       string    `json:"bodyExample"`
	ResponseExample   string    `json:"responseExample"`
	PricingMode       string    `json:"pricingMode"` // free | paid
	Price             float64   `json:"price"`
	PriceUnit         string    `json:"priceUnit"`
	UpstreamPricing   string    `json:"upstreamPricing"`   // 上游计费状态快照: free | paid | ''（未同步）
	UpstreamPrice     float64   `json:"upstreamPrice"`     // 上游标价
	UpstreamPriceUnit string    `json:"upstreamPriceUnit"` // 上游计价单位
	FreeQuota         int       `json:"freeQuota"`
	RequiresLogin     bool      `json:"requiresLogin"`
	Status            string    `json:"status"` // online | offline | maintenance
	LatencyMs         int       `json:"latencyMs"`
	Popularity        int       `json:"popularity"`
	Tags              []string  `json:"tags"`
	Sort              int       `json:"sort"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// APIKey 用户调用本站网关用的密钥
type APIKey struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"userId"`
	Username   string     `json:"username"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"` // sk-xxxx 前 10 位，用于展示
	Status     string     `json:"status"` // active | disabled
	Quota      int        `json:"quota"`  // 每日调用上限，0 表示不限
	UsedToday  int        `json:"usedToday"`
	UsedTotal  int64      `json:"usedTotal"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
}

// Usage 网关调用记录
type Usage struct {
	ID       int64     `json:"id"`
	KeyID    int64     `json:"keyId"`
	UserID   int64     `json:"userId"`
	Username string    `json:"username"`
	Slug     string    `json:"slug"`
	Path     string    `json:"path"`
	Method   string    `json:"method"`
	Status   int       `json:"status"`
	Ms       int64     `json:"ms"`
	IP       string    `json:"ip"`
	Stream   bool      `json:"stream"`
	TS       time.Time `json:"ts"`
}

// User 后台/前台账号
type User struct {
	ID             int64      `json:"id"`
	Username       string     `json:"username"`
	Role           string     `json:"role"` // admin | user
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"createdAt"`
	LastLogin      *time.Time `json:"lastLogin"`
	Note           string     `json:"note"`
	MustChangePass bool       `json:"mustChangePass"` // 使用默认密码，登录后强制修改
}

// Voice 克隆音色
type Voice struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Owner      string    `json:"owner"`
	Mime       string    `json:"mime"`
	Size       int64     `json:"size"`
	DurationMs int       `json:"durationMs"`
	Source     string    `json:"source"` // upload | record
	Note       string    `json:"note"`
	CreatedAt  time.Time `json:"createdAt"`
	HasAudio   bool      `json:"hasAudio"`
}

// Render 合成记录
type Render struct {
	ID         int64     `json:"id"`
	Owner      string    `json:"owner"`
	VoiceID    int64     `json:"voiceId"`
	VoiceName  string    `json:"voiceName"`
	Text       string    `json:"text"`
	Style      string    `json:"style"`
	Format     string    `json:"format"`
	Size       int64     `json:"size"`
	DurationMs int       `json:"durationMs"`
	CreatedAt  time.Time `json:"createdAt"`
}

// AdSlot 广告位
type AdSlot struct {
	Slot    string `json:"slot"`
	Name    string `json:"name"`
	HTML    string `json:"html"`
	Enabled bool   `json:"enabled"`
}

// LogEntry 访问日志
type LogEntry struct {
	ID     int64     `json:"id"`
	TS     time.Time `json:"ts"`
	IP     string    `json:"ip"`
	Method string    `json:"method"`
	Path   string    `json:"path"`
	Status int       `json:"status"`
	Ms     int64     `json:"ms"`
	Kind   string    `json:"kind"`
	UA     string    `json:"ua"`
}

// TTSConfig 音色克隆相关配置
type TTSConfig struct {
	Enabled       bool   `json:"enabled"`
	Provider      string `json:"provider"`
	BaseURL       string `json:"baseUrl"`
	APIKey        string `json:"apiKey"`
	Model         string `json:"model"`
	DesignModel   string `json:"designModel"`
	PresetModel   string `json:"presetModel"`
	DefaultVoice  string `json:"defaultVoice"`
	MaxUploadMB   int    `json:"maxUploadMb"`
	MaxChars      int    `json:"maxChars"`
	TimeoutSec    int    `json:"timeoutSec"`
	GuestAllowed  bool   `json:"guestAllowed"`
	KeepRenderDay int    `json:"keepRenderDays"`
}

// Settings 全站配置
type Settings struct {
	SiteName        string    `json:"siteName"`
	SiteTagline     string    `json:"siteTagline"`
	SiteDesc        string    `json:"siteDesc"`
	LogoText        string    `json:"logoText"`
	ICP             string    `json:"icp"`
	FooterText      string    `json:"footerText"`
	Announcement    string    `json:"announcement"`
	ShowAnnounce    bool      `json:"showAnnouncement"`
	UpstreamBase    string    `json:"upstreamBase"`
	UpstreamPrefix  string    `json:"upstreamPrefix"`
	UpstreamKey     string    `json:"upstreamKey"`
	UpstreamAuthHdr string    `json:"upstreamAuthHeader"`
	UpstreamAuthPfx string    `json:"upstreamAuthPrefix"`
	// 抖音数据服务（旁挂 Douyin_TikTok_Download_API）：
	// 「抖音作者作品分析」不走上游转发，由本站直连这个旁挂服务翻页聚合
	DouyinSidecar    string `json:"douyinSidecar"`
	DouyinSidecarKey string `json:"douyinSidecarKey"`
	GatewayEnabled  bool      `json:"gatewayEnabled"`
	PublicBase      string    `json:"publicBase"`
	DefaultQuota    int       `json:"defaultQuota"`
	GuestDailyQuota int       `json:"guestDailyQuota"`
	PlaygroundKey   string    `json:"playgroundKey"`
	AllowGuest      bool      `json:"allowGuest"`
	AllowPlayground bool      `json:"allowPlayground"`
	RequireLoginAll bool      `json:"requireLoginAll"`
	RegisterOpen    bool      `json:"registerOpen"`
	Maintenance     bool      `json:"maintenance"`
	MaintenanceMsg  string    `json:"maintenanceMsg"`
	ShowStats       bool      `json:"showStats"`
	RateLimitPerMin int       `json:"rateLimitPerMin"`
	RateLimitBurst  int       `json:"rateLimitBurst"`
	MaxBodyMB       int       `json:"maxBodyMb"`
	BanMinutes      int       `json:"banMinutes"`
	AdminIPAllow    []string  `json:"adminIpAllow"`
	TTS             TTSConfig `json:"tts"`
}

// DefaultSettings 返回出厂配置
func DefaultSettings() Settings {
	return Settings{
		SiteName:        "清影 ClearFrame",
		SiteTagline:     "粘贴链接，原画直出",
		SiteDesc:        "粘贴分享链接，自动识别平台并解析出无水印原画视频、图集与实况照片，支持抖音、快手、B站、视频号等 30+ 站点。",
		LogoText:        "清",
		FooterText:      "© 2026 清影 ClearFrame · 仅供个人学习与研究使用",
		Announcement:    "已接入短视频解析聚合通道，覆盖 30+ 平台；部分能力来自第三方平台，仅供学习研究，请勿用于商业用途。",
		ShowAnnounce:    true,
		UpstreamBase:    "", // 部署后在后台「全局设置 → 上游接入」填写，不内置具体上游地址
		UpstreamPrefix:  "/api",
		UpstreamAuthHdr: "x-api-key",
		UpstreamAuthPfx: "",
		GatewayEnabled:  true,
		DefaultQuota:    1000,
		GuestDailyQuota: 20,
		AllowGuest:      true,
		AllowPlayground: true,
		RegisterOpen:    true,
		ShowStats:       true,
		RateLimitPerMin: 120,
		RateLimitBurst:  40,
		MaxBodyMB:       4,
		BanMinutes:      15,
		MaintenanceMsg:  "服务升级中，预计 10 分钟后恢复。",
		TTS: TTSConfig{
			Enabled:       true,
			Provider:      "mimo",
			BaseURL:       "https://api.xiaomimimo.com/v1",
			Model:         "mimo-v2.5-tts-voiceclone",
			DesignModel:   "mimo-v2.5-tts-voicedesign",
			PresetModel:   "mimo-v2.5-tts",
			DefaultVoice:  "冰糖",
			MaxUploadMB:   2,
			MaxChars:      2000,
			TimeoutSec:    90,
			GuestAllowed:  true,
			KeepRenderDay: 30,
		},
	}
}
