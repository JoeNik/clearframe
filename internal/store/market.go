package store

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 上游字段类型不稳定（数字/字符串混用），统一用宽松类型接收
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" || s == "false" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexInt(n)
	return nil
}

type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexFloat(n)
	return nil
}

type flexStr string

func (f *flexStr) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*f = ""
		return nil
	}
	s = strings.Trim(s, `"`)
	*f = flexStr(s)
	return nil
}

// 上游接口市场：/system-api/market/apis 是公开的接口清单，
// 每条记录带 gatewayPath、请求参数、响应示例、计费与 QPS 限制，
// 足够直接生成本站的接口目录。

type marketParam struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	Location     string `json:"location"`
	Required     bool   `json:"required"`
	Description  string `json:"description"`
	Placeholder  string `json:"placeholder"`
	ExampleValue string `json:"exampleValue"`
}

type marketAPI struct {
	ID                 flexStr         `json:"id"`
	APICode            string          `json:"apiCode"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	Remark             string          `json:"remark"`
	RequestMethod      string          `json:"requestMethod"`
	GatewayPath        string          `json:"gatewayPath"`
	GatewayURLTemplate string          `json:"gatewayUrlTemplate"`
	DocSummary         string          `json:"docSummary"`
	DocNotice          string          `json:"docNotice"`
	DocRequestParams   json.RawMessage `json:"docRequestParams"`
	DocResponseExample json.RawMessage `json:"docResponseExample"`
	Price              flexFloat       `json:"price"`
	PriceLabel         string          `json:"priceLabel"`
	BillingEnabled     flexInt         `json:"billingEnabled"`
	Status             flexInt         `json:"status"`
	CallCount          flexInt         `json:"callCount"`
	QPSLimit           flexInt         `json:"qpsLimit"`
	IsTop              flexInt         `json:"isTop"`
	IsFeatured         flexInt         `json:"isFeatured"`
	ResponseType       string          `json:"responseType"`
}

// decodeLoose 兼容「字段是 JSON 文本」与「字段已是对象/数组」两种形态
func decodeLoose(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return fmt.Errorf("empty")
	}
	if err := json.Unmarshal(raw, dst); err == nil {
		return nil
	}
	var str string
	if err := json.Unmarshal(raw, &str); err != nil {
		return err
	}
	str = strings.TrimSpace(str)
	if str == "" || str == "null" {
		return fmt.Errorf("empty")
	}
	return json.Unmarshal([]byte(str), dst)
}

type marketResp struct {
	Code int `json:"code"`
	Data struct {
		Items []marketAPI `json:"items"`
		Total int         `json:"total"`
	} `json:"data"`
}

// 接口分组：按 apiCode 归类，未知的归入「其他解析」
func categoryOf(code string) (slug, name string) {
	switch code {
	case "dytj":
		return "data", "数据分析"
	case "domailcx", "bbhj":
		return "tool", "实用工具"
	default:
		return "parse", "视频解析"
	}
}

// SyncFromMarket 从上游拉取接口清单并写入本地库。
// base 形如 https://upstream.example.com（不含末尾斜杠）。
// 返回同步的接口数量；已存在的按 apiCode 覆盖，不删除本地独有条目。
func (s *Store) SyncFromMarket(base string) (int, []string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return 0, nil, fmt.Errorf("未配置上游地址")
	}
	url := base + "/system-api/market/apis"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ClearFrame-Sync/1.0")

	client := &http.Client{
		Timeout: 25 * time.Second,
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout: 15 * time.Second,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("请求上游失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, nil, fmt.Errorf("上游返回 %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return 0, nil, err
	}
	var mr marketResp
	if err := json.Unmarshal(raw, &mr); err != nil {
		return 0, nil, fmt.Errorf("解析上游响应失败: %w", err)
	}
	if mr.Code != 0 || len(mr.Data.Items) == 0 {
		return 0, nil, fmt.Errorf("上游未返回接口清单（code=%d）", mr.Code)
	}

	// 保证分类存在
	for _, c := range []Category{
		{Slug: "parse", Name: "视频解析", Icon: "video", Sort: 10},
		{Slug: "data", Name: "数据分析", Icon: "gauge", Sort: 20},
		{Slug: "tool", Name: "实用工具", Icon: "tool", Sort: 30},
	} {
		if _, err := s.UpsertCategory(c); err != nil {
			return 0, nil, err
		}
	}

	items := mr.Data.Items
	// 按调用量排序，热门的排前面
	sort.SliceStable(items, func(i, j int) bool { return items[i].CallCount > items[j].CallCount })
	maxCalls := int64(1)
	for _, it := range items {
		if int64(it.CallCount) > maxCalls {
			maxCalls = int64(it.CallCount)
		}
	}

	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()

	warnings := []string{}
	n := 0
	for idx, it := range items {
		code := strings.TrimSpace(it.APICode)
		if code == "" {
			continue
		}
		catSlug, _ := categoryOf(code)

		// 参数
		params := []Param{}
		if len(it.DocRequestParams) > 0 {
			var mp []marketParam
			if err := decodeLoose(it.DocRequestParams, &mp); err == nil {
				for _, p := range mp {
					if p.Name == "" {
						continue
					}
					desc := p.Description
					if desc == "" {
						desc = p.Placeholder
					}
					def := ""
					if p.ExampleValue != "" {
						def = p.ExampleValue
					}
					params = append(params, Param{
						Name: p.Name, Type: p.Type, Required: p.Required,
						Default: def, Desc: desc,
					})
				}
			} else {
				warnings = append(warnings, code+": 参数解析失败")
			}
		}

		// 请求示例：GET 用 query 串，POST 用 JSON body
		method := strings.ToUpper(strings.TrimSpace(it.RequestMethod))
		switch method {
		case "GET_POST", "POST_GET", "ALL":
			method = "GET"
		case "":
			method = "GET"
		}
		bodyExample := buildParamExample(method, params)
		if bodyExample == "" {
			bodyExample = ""
		}

		respExample := ""
		if len(it.DocResponseExample) > 0 {
			var v any
			if err := decodeLoose(it.DocResponseExample, &v); err == nil {
				if b, err := json.MarshalIndent(v, "", "  "); err == nil {
					respExample = string(b)
					if len(respExample) > 6000 {
						respExample = respExample[:6000] + "\n…（示例过长已截断）"
					}
				}
			}
		}

		desc := strings.TrimSpace(it.DocSummary)
		if desc == "" {
			desc = strings.TrimSpace(it.Description)
		}
		if notice := strings.TrimSpace(it.DocNotice); notice != "" {
			desc += "\n\n" + notice
		}

		status := "online"
		if it.Status != 1 {
			status = "offline"
		}
		// 上游计费状态快照：billingEnabled=1 表示上游开启计费
		// （即使单价为 0，也可能是按量/按用户等级计价），price>0 为明码标价。
		// 只写 upstream_pricing/price/price_unit，不覆盖本地计费设置
		upPricing := "free"
		if it.BillingEnabled == 1 || float64(it.Price) > 0 {
			upPricing = "paid"
		}
		upUnit := "次"
		// 从上游价格说明推导计价单位（如 "0.01元/次" → "次"）
		if lab := strings.TrimSpace(it.PriceLabel); lab != "" {
			if i := strings.Index(lab, "/"); i >= 0 && i+1 < len(lab) {
				upUnit = strings.TrimSpace(lab[i+1:])
			}
		}

		upPath := strings.TrimSpace(it.GatewayPath)
		if upPath == "" {
			upPath = strings.TrimSpace(it.GatewayURLTemplate)
		}
		if upPath == "" {
			upPath = "/api/" + code
		}

		popularity := int(int64(it.CallCount) * 1000 / maxCalls)
		tags := []string{"去水印"}
		if it.IsFeatured == 1 {
			tags = append(tags, "精选")
		}
		if it.IsTop == 1 {
			tags = append(tags, "热门")
		}
		// 免费/收费不再写进 tags：计费状态由本地 pricing_mode 与 upstream_pricing 表达，
		// 这里顺带清理老版本同步写入的同类标签
		clean := tags[:0]
		for _, t := range tags {
			if t == "免费" || t == "收费" {
				continue
			}
			clean = append(clean, t)
		}
		tags = clean

		paramsJSON, _ := json.Marshal(params)

		// 检测已有记录，用于「上游转收费 → 本地计费自动开启」提醒
		var existMode string
		hasExist := false
		if e := tx.QueryRow(`SELECT pricing_mode FROM interfaces WHERE slug = ?`, code).Scan(&existMode); e == nil {
			hasExist = true
		}

		// 写入规则：
		// - 新记录：本地计费字段用上游状态初始化（尚无本地设置）
		// - 已有记录：upstream_* 快照始终更新；本地计费仅在上游收费时自动开启
		//   （上游 price>0 时连同单价/单位一起同步，=0 时只开计费、保留本地单价）
		// - 上游免费时不回改本地设置（管理员可以继续对免费上游自行定价）
		if hasExist && existMode != "paid" && upPricing == "paid" {
			if float64(it.Price) > 0 {
				warnings = append(warnings, fmt.Sprintf("%s: 检测到上游已收费，本地计费已自动开启（¥%s/%s）",
					code, trimZeros(float64(it.Price)), upUnit))
			} else {
				warnings = append(warnings, fmt.Sprintf("%s: 检测到上游已收费，本地计费已自动开启", code))
			}
		}

		_, err := tx.Exec(`INSERT INTO interfaces(slug, name, category, method, path, upstream_path,
			upstream_method, summary, description, params, body_example, response_example,
			pricing_mode, price, price_unit, upstream_pricing, upstream_price, upstream_price_unit,
			free_quota, requires_login, status, latency_ms,
			popularity, tags, sort, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(slug) DO UPDATE SET
				name=excluded.name, category=excluded.category, method=excluded.method,
				path=excluded.path, upstream_path=excluded.upstream_path,
				summary=excluded.summary, description=excluded.description,
				params=excluded.params, body_example=excluded.body_example,
				response_example=excluded.response_example,
				upstream_pricing=excluded.upstream_pricing, upstream_price=excluded.upstream_price,
				upstream_price_unit=excluded.upstream_price_unit,
				pricing_mode=CASE WHEN excluded.upstream_pricing='paid' THEN 'paid' ELSE interfaces.pricing_mode END,
				price=CASE WHEN excluded.upstream_pricing='paid' AND excluded.upstream_price>0
					THEN excluded.upstream_price ELSE interfaces.price END,
				price_unit=CASE WHEN excluded.upstream_pricing='paid' AND excluded.upstream_price>0
					THEN excluded.upstream_price_unit ELSE interfaces.price_unit END,
				status=excluded.status, popularity=excluded.popularity,
				tags=excluded.tags, sort=excluded.sort, updated_at=excluded.updated_at`,
			code, it.Name, catSlug, method, "/v1/"+code, upPath,
			method, firstLine(it.Description, it.DocSummary), desc, string(paramsJSON),
			bodyExample, respExample, upPricing, float64(it.Price), upUnit,
			upPricing, float64(it.Price), upUnit, 0, 0, status, 0,
			popularity, strings.Join(tags, ","), (idx+1)*10, now, now)
		if err != nil {
			return n, warnings, fmt.Errorf("写入 %s 失败: %w", code, err)
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return n, warnings, err
	}
	return n, warnings, nil
}

// buildParamExample 依据参数表生成请求示例
func buildParamExample(method string, params []Param) string {
	if len(params) == 0 {
		return ""
	}
	if method == "POST" {
		obj := map[string]any{}
		keys := []string{}
		for _, p := range params {
			if !p.Required {
				continue
			}
			keys = append(keys, p.Name)
			obj[p.Name] = p.Default
			if obj[p.Name] == "" {
				obj[p.Name] = "string"
			}
		}
		if len(obj) == 0 {
			return ""
		}
		sort.Strings(keys)
		b, _ := json.MarshalIndent(obj, "", "  ")
		return string(b)
	}
	// GET：拼 query
	parts := []string{}
	for _, p := range params {
		if !p.Required {
			continue
		}
		v := p.Default
		if v == "" {
			v = "value"
		}
		parts = append(parts, p.Name+"="+v)
	}
	return strings.Join(parts, "&")
}

func firstLine(a, b string) string {
	s := strings.TrimSpace(a)
	if s == "" {
		s = strings.TrimSpace(b)
	}
	if i := strings.IndexAny(s, "\n。；;"); i > 0 {
		s = s[:i]
	}
	if len([]rune(s)) > 60 {
		s = string([]rune(s)[:60])
	}
	return s
}

// trimZeros 去掉浮点数多余的 0（0.050 → 0.05，用于提示文案）
func trimZeros(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return s
}
