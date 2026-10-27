package store

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// 验证上游计费状态同步：
// 1. 新记录：本地计费字段用上游状态初始化，upstream_* 快照写入
// 2. 上游转收费：本地计费自动开启（单价>0 时跟随上游单价），并产生提醒
// 3. 上游免费：不回改本地设置（管理员可自行定价）
// 4. tags 不再包含 免费/收费 标签
func TestSyncFromMarketPricing(t *testing.T) {
	var mu sync.Mutex
	upPrice := 0.05
	upLabel := "0.05元/千次"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		price, label := upPrice, upLabel
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{"items":[
			{"apiCode":"tfree","name":"免费接口","status":1,"price":0,"priceLabel":"免费","billingEnabled":0,"callCount":1},
			{"apiCode":"tpaid","name":"收费接口","status":1,"price":`+strconv.FormatFloat(price, 'f', -1, 64)+`,"priceLabel":"`+label+`","billingEnabled":1,"callCount":2},
			{"apiCode":"tbill","name":"按量计费","status":1,"price":0,"priceLabel":"","billingEnabled":1,"callCount":3}
		]}}`)
	}))
	defer srv.Close()

	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// ---- 第一次同步：全部是新记录，本地计费由上游初始化 ----
	n, warns, err := s.SyncFromMarket(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || len(warns) != 0 {
		t.Fatalf("synced=%d warns=%v", n, warns)
	}

	cases := []struct {
		slug   string
		mode   string
		upMode string
		upPx   float64
		upUnit string
	}{
		{"tfree", "free", "free", 0, "次"},
		{"tpaid", "paid", "paid", 0.05, "千次"},
		{"tbill", "paid", "paid", 0, "次"},
	}
	for _, c := range cases {
		it, err := s.GetInterface(c.slug)
		if err != nil {
			t.Fatalf("%s: %v", c.slug, err)
		}
		if it.PricingMode != c.mode {
			t.Errorf("%s: pricing_mode=%s want %s", c.slug, it.PricingMode, c.mode)
		}
		if it.UpstreamPricing != c.upMode {
			t.Errorf("%s: upstream_pricing=%s want %s", c.slug, it.UpstreamPricing, c.upMode)
		}
		if it.UpstreamPrice != c.upPx {
			t.Errorf("%s: upstream_price=%v want %v", c.slug, it.UpstreamPrice, c.upPx)
		}
		if it.UpstreamPriceUnit != c.upUnit {
			t.Errorf("%s: upstream_price_unit=%s want %s", c.slug, it.UpstreamPriceUnit, c.upUnit)
		}
		for _, tag := range it.Tags {
			if tag == "免费" || tag == "收费" {
				t.Errorf("%s: tags 不应再包含计费标签，得到 %v", c.slug, it.Tags)
			}
		}
	}

	// ---- 管理员本地调整计费设置 ----
	// tpaid：本地改免费（模拟管理员手动关闭）
	itPaid, err := s.GetInterface("tpaid")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PatchInterface(itPaid.ID, map[string]any{"pricingMode": "free", "price": 1.5}); err != nil {
		t.Fatal(err)
	}
	// tfree：上游免费但管理员自行定价收费
	itFree, err := s.GetInterface("tfree")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PatchInterface(itFree.ID, map[string]any{"pricingMode": "paid", "price": 0.9}); err != nil {
		t.Fatal(err)
	}

	// ---- 上游涨价后再次同步 ----
	// 预期：tpaid 本地计费自动重新开启并跟随上游单价；tfree 上游仍免费、本地定价保留
	mu.Lock()
	upPrice = 0.08
	upLabel = "0.08元/千次"
	mu.Unlock()
	_, warns2, err := s.SyncFromMarket(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	it2, err := s.GetInterface("tpaid")
	if err != nil {
		t.Fatal(err)
	}
	if it2.PricingMode != "paid" || it2.Price != 0.08 || it2.UpstreamPriceUnit != "千次" {
		t.Errorf("上游收费未联动本地: mode=%s price=%v unit=%s（应 paid/0.08/千次）",
			it2.PricingMode, it2.Price, it2.UpstreamPriceUnit)
	}
	if it2.UpstreamPricing != "paid" || it2.UpstreamPrice != 0.08 {
		t.Errorf("上游快照未更新: mode=%s price=%v（应 paid/0.08）", it2.UpstreamPricing, it2.UpstreamPrice)
	}

	it3, err := s.GetInterface("tfree")
	if err != nil {
		t.Fatal(err)
	}
	if it3.PricingMode != "paid" || it3.Price != 0.9 {
		t.Errorf("上游免费时不应回改本地设置: mode=%s price=%v（应保留 paid/0.9）", it3.PricingMode, it3.Price)
	}

	// 自动开启提醒应出现在 warnings 里
	found := false
	for _, w := range warns2 {
		if strings.Contains(w, "tpaid") && strings.Contains(w, "自动开启") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings 缺少自动开启提醒: %v", warns2)
	}
}
