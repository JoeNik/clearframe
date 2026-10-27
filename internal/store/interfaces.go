package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ErrNotFound 记录不存在
var ErrNotFound = errors.New("记录不存在")

// ---------- categories ----------

// Categories 返回全部分类
func (s *Store) Categories() ([]Category, error) {
	rows, err := s.db.Query(`SELECT id, slug, name, icon, sort FROM categories ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Slug, &c.Name, &c.Icon, &c.Sort); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpsertCategory 新增或更新分类
func (s *Store) UpsertCategory(c Category) (Category, error) {
	if c.Slug == "" {
		return c, errors.New("分类标识不能为空")
	}
	_, err := s.db.Exec(`INSERT INTO categories(slug, name, icon, sort) VALUES(?,?,?,?)
		ON CONFLICT(slug) DO UPDATE SET name = excluded.name, icon = excluded.icon, sort = excluded.sort`,
		c.Slug, c.Name, c.Icon, c.Sort)
	if err != nil {
		return c, err
	}
	err = s.db.QueryRow(`SELECT id, slug, name, icon, sort FROM categories WHERE slug = ?`, c.Slug).
		Scan(&c.ID, &c.Slug, &c.Name, &c.Icon, &c.Sort)
	return c, err
}

// DeleteCategory 删除分类
func (s *Store) DeleteCategory(slug string) error {
	_, err := s.db.Exec(`DELETE FROM categories WHERE slug = ?`, slug)
	return err
}

// ---------- interfaces ----------

const ifaceCols = `id, slug, name, category, method, path, upstream_path, upstream_method, summary, description, params,
	body_example, response_example, pricing_mode, price, price_unit, free_quota,
	upstream_pricing, upstream_price, upstream_price_unit,
	requires_login, status, latency_ms, popularity, tags, sort, created_at, updated_at`

func scanIface(sc interface{ Scan(...any) error }) (Interface, error) {
	var it Interface
	var params, tags string
	var created, updated int64
	var requiresLogin int
	err := sc.Scan(&it.ID, &it.Slug, &it.Name, &it.Category, &it.Method, &it.Path,
		&it.UpstreamPath, &it.UpstreamMethod, &it.Summary,
		&it.Description, &params, &it.BodyExample, &it.ResponseExample, &it.PricingMode,
		&it.Price, &it.PriceUnit, &it.FreeQuota,
		&it.UpstreamPricing, &it.UpstreamPrice, &it.UpstreamPriceUnit,
		&requiresLogin, &it.Status, &it.LatencyMs,
		&it.Popularity, &tags, &it.Sort, &created, &updated)
	if err != nil {
		return it, err
	}
	it.RequiresLogin = requiresLogin != 0
	it.Params = []Param{}
	_ = json.Unmarshal([]byte(params), &it.Params)
	it.Tags = splitTags(tags)
	it.CreatedAt = time.Unix(created, 0)
	it.UpdatedAt = time.Unix(updated, 0)
	return it, nil
}

// ListInterfaces 按条件列出接口。status 为空表示全部。
func (s *Store) ListInterfaces(status, category, keyword string, limit, offset int) ([]Interface, int, error) {
	where := []string{"1=1"}
	args := []any{}
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	if category != "" {
		where = append(where, "category = ?")
		args = append(args, category)
	}
	if keyword != "" {
		where = append(where, "(name LIKE ? OR slug LIKE ? OR path LIKE ? OR summary LIKE ? OR tags LIKE ?)")
		k := "%" + keyword + "%"
		args = append(args, k, k, k, k, k)
	}
	clause := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM interfaces WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 200
	}
	q := `SELECT ` + ifaceCols + ` FROM interfaces WHERE ` + clause +
		` ORDER BY sort, popularity DESC, id ASC LIMIT ? OFFSET ?`
	rows, err := s.db.Query(q, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Interface{}
	for rows.Next() {
		it, err := scanIface(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, rows.Err()
}

// GetInterface 按 slug 获取
func (s *Store) GetInterface(slug string) (Interface, error) {
	row := s.db.QueryRow(`SELECT `+ifaceCols+` FROM interfaces WHERE slug = ?`, slug)
	it, err := scanIface(row)
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

// GetInterfaceByPath 按对外路径查找（网关路由用，优先精确匹配在线接口）
func (s *Store) GetInterfaceByPath(path string) (Interface, error) {
	row := s.db.QueryRow(`SELECT `+ifaceCols+` FROM interfaces WHERE path = ? ORDER BY sort, id LIMIT 1`, path)
	it, err := scanIface(row)
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

// GetInterfaceByID 按 ID 获取
func (s *Store) GetInterfaceByID(id int64) (Interface, error) {
	row := s.db.QueryRow(`SELECT `+ifaceCols+` FROM interfaces WHERE id = ?`, id)
	it, err := scanIface(row)
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

// SaveInterface 新增或更新接口
func (s *Store) SaveInterface(it Interface) (Interface, error) {
	if strings.TrimSpace(it.Slug) == "" {
		return it, errors.New("接口标识不能为空")
	}
	if it.Method == "" {
		it.Method = "POST"
	}
	it.Method = strings.ToUpper(it.Method)
	if it.Status == "" {
		it.Status = "online"
	}
	if it.PricingMode != "paid" {
		it.PricingMode = "free"
	}
	if it.PriceUnit == "" {
		it.PriceUnit = "次"
	}
	params, _ := json.Marshal(it.Params)
	now := time.Now().Unix()

	if it.ID > 0 {
		_, err := s.db.Exec(`UPDATE interfaces SET slug=?, name=?, category=?, method=?, path=?,
			upstream_path=?, upstream_method=?, summary=?, description=?, params=?, body_example=?, response_example=?, pricing_mode=?,
			price=?, price_unit=?, free_quota=?, requires_login=?, status=?, latency_ms=?,
			popularity=?, tags=?, sort=?, updated_at=? WHERE id=?`,
			it.Slug, it.Name, it.Category, it.Method, it.Path, it.UpstreamPath, it.UpstreamMethod, it.Summary, it.Description,
			string(params), it.BodyExample, it.ResponseExample, it.PricingMode, it.Price,
			it.PriceUnit, it.FreeQuota, boolInt(it.RequiresLogin), it.Status, it.LatencyMs,
			it.Popularity, strings.Join(it.Tags, ","), it.Sort, now, it.ID)
		if err != nil {
			return it, err
		}
	} else {
		res, err := s.db.Exec(`INSERT INTO interfaces(slug, name, category, method, path, upstream_path, upstream_method, summary,
			description, params, body_example, response_example, pricing_mode, price, price_unit,
			free_quota, requires_login, status, latency_ms, popularity, tags, sort, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			it.Slug, it.Name, it.Category, it.Method, it.Path, it.UpstreamPath, it.UpstreamMethod, it.Summary, it.Description,
			string(params), it.BodyExample, it.ResponseExample, it.PricingMode, it.Price,
			it.PriceUnit, it.FreeQuota, boolInt(it.RequiresLogin), it.Status, it.LatencyMs,
			it.Popularity, strings.Join(it.Tags, ","), it.Sort, now, now)
		if err != nil {
			return it, err
		}
		id, _ := res.LastInsertId()
		it.ID = id
	}
	return s.GetInterface(it.Slug)
}

// PatchInterface 局部更新（用于列表内快速开关）
func (s *Store) PatchInterface(id int64, fields map[string]any) error {
	allowed := map[string]string{
		"status":         "status",
		"pricingMode":    "pricing_mode",
		"requiresLogin":  "requires_login",
		"price":          "price",
		"freeQuota":      "free_quota",
		"sort":           "sort",
		"name":           "name",
		"summary":        "summary",
		"category":       "category",
		"popularity":     "popularity",
		"latencyMs":      "latency_ms",
		"upstreamPath":   "upstream_path",
		"upstreamMethod": "upstream_method",
	}
	sets := []string{}
	args := []any{}
	for k, v := range fields {
		col, ok := allowed[k]
		if !ok {
			continue
		}
		if b, isBool := v.(bool); isBool {
			v = boolInt(b)
		}
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if len(sets) == 0 {
		return errors.New("没有可更新的字段")
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().Unix(), id)
	_, err := s.db.Exec(`UPDATE interfaces SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	return err
}

// DeleteInterface 删除接口
func (s *Store) DeleteInterface(id int64) error {
	_, err := s.db.Exec(`DELETE FROM interfaces WHERE id = ?`, id)
	return err
}

// BulkReplaceInterfaces 批量导入（按 slug 覆盖）
func (s *Store) BulkReplaceInterfaces(list []Interface, wipe bool) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if wipe {
		if _, err := tx.Exec(`DELETE FROM interfaces`); err != nil {
			return 0, err
		}
	}
	n := 0
	for _, it := range list {
		if strings.TrimSpace(it.Slug) == "" {
			continue
		}
		if it.Method == "" {
			it.Method = "POST"
		}
		if it.Status == "" {
			it.Status = "online"
		}
		if it.PricingMode != "paid" {
			it.PricingMode = "free"
		}
		if it.PriceUnit == "" {
			it.PriceUnit = "次"
		}
		params, _ := json.Marshal(it.Params)
		now := time.Now().Unix()
		_, err := tx.Exec(`INSERT INTO interfaces(slug, name, category, method, path, upstream_path, upstream_method, summary,
			description, params, body_example, response_example, pricing_mode, price, price_unit,
			free_quota, requires_login, status, latency_ms, popularity, tags, sort, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(slug) DO UPDATE SET name=excluded.name, category=excluded.category,
			method=excluded.method, path=excluded.path, upstream_path=excluded.upstream_path,
			upstream_method=excluded.upstream_method, summary=excluded.summary,
			description=excluded.description, params=excluded.params, body_example=excluded.body_example,
			response_example=excluded.response_example, pricing_mode=excluded.pricing_mode,
			price=excluded.price, price_unit=excluded.price_unit, free_quota=excluded.free_quota,
			requires_login=excluded.requires_login, status=excluded.status, latency_ms=excluded.latency_ms,
			tags=excluded.tags, sort=excluded.sort, updated_at=excluded.updated_at`,
			it.Slug, it.Name, it.Category, it.Method, it.Path, it.UpstreamPath, it.UpstreamMethod, it.Summary, it.Description,
			string(params), it.BodyExample, it.ResponseExample, it.PricingMode, it.Price,
			it.PriceUnit, it.FreeQuota, boolInt(it.RequiresLogin), it.Status, it.LatencyMs,
			it.Popularity, strings.Join(it.Tags, ","), it.Sort, now, now)
		if err != nil {
			return n, err
		}
		n++
	}
	return n, tx.Commit()
}

// Stats 汇总统计
type Stats struct {
	Total      int            `json:"total"`
	Online     int            `json:"online"`
	Free       int            `json:"free"`
	Paid       int            `json:"paid"`
	NeedLogin  int            `json:"needLogin"`
	Categories map[string]int `json:"categories"`
	Calls      int64          `json:"calls"`
	Today      int64          `json:"today"`
}

// Stats 计算概览数据
func (s *Store) Stats() Stats {
	st := Stats{Categories: map[string]int{}}
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM interfaces`).Scan(&st.Total)
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM interfaces WHERE status = 'online'`).Scan(&st.Online)
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM interfaces WHERE pricing_mode = 'free'`).Scan(&st.Free)
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM interfaces WHERE pricing_mode = 'paid'`).Scan(&st.Paid)
	_ = s.db.QueryRow(`SELECT COUNT(1) FROM interfaces WHERE requires_login = 1`).Scan(&st.NeedLogin)
	rows, err := s.db.Query(`SELECT category, COUNT(1) FROM interfaces GROUP BY category`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var c string
			var n int
			if rows.Scan(&c, &n) == nil {
				st.Categories[c] = n
			}
		}
	}
	st.Calls = s.Counter("calls_total")
	st.Today = s.Counter("calls:" + time.Now().Format("2006-01-02"))
	return st
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
