package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// 合成模式
const (
	ModePreset = "preset"
	ModeClone  = "clone"
	ModeDesign = "design"
)

// PresetVoices 官方预置音色
var PresetVoices = []struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Lang string `json:"lang"`
	Sex  string `json:"sex"`
}{
	{"mimo_default", "默认", "zh", "女"},
	{"冰糖", "冰糖", "zh", "女"},
	{"茉莉", "茉莉", "zh", "女"},
	{"苏打", "苏打", "zh", "男"},
	{"白桦", "白桦", "zh", "男"},
	{"Mia", "Mia", "en", "女"},
	{"Chloe", "Chloe", "en", "女"},
	{"Milo", "Milo", "en", "男"},
	{"Dean", "Dean", "en", "男"},
}

// StylePresets 克隆常用风格提示词。克隆模式下这些描述会显著影响还原度与语气。
var StylePresets = []struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prompt string `json:"prompt"`
}{
	{"none", "贴近原声", ""},
	{"fast", "语速快", "语速快，一个字紧接一个字不拖沓。用与参考音频相同的音色，吐字紧凑，节奏往前赶，其余语气尽量贴近原声。"},
	{"turbo", "语速极快", "语速极快、干脆利落。用与参考音频相同的音色，一口气把话说完，句间不拖长，贴近原声音色。"},
	{"emo", "情绪起伏大", "用与参考音频相同的音色说话，但情绪起伏明显更大：平静处低缓沉稳，动情处音调明显上扬、尾音拉长，关键词重重强调，整体音调大开大合，更有感染力。"},
	{"fast_emo", "语速快 + 大情绪", "语速快，一个字紧接一个字。用与参考音频相同的音色，情绪起伏明显更大：平静处低缓，动情处音调上扬加重，大开大合，又快又有感染力。"},
	{"soft", "温柔低语", "用与参考音频相同的音色，轻声、温柔地说话，气息柔和，语速偏慢，句尾自然收住。"},
}

// ReferenceVoice 参考音色目录：官方预置音色 + 原创设计风格。
// 设计音色全部由文字描述生成，是本站原创的人设风格，
// 不模仿、不冒充任何真实人物（真人声音受声音权益保护，未经授权不得克隆）。
type ReferenceVoice struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"` // preset | design
	Name     string `json:"name"`
	Desc     string `json:"desc"`
	Sex      string `json:"sex"`
	Lang     string `json:"lang"`
	Voice    string `json:"voice,omitempty"` // preset：官方音色名
	Hint     string `json:"hint,omitempty"`  // design：音色描述
	Sentence string `json:"sentence"`        // 试听台词
	Demo     bool   `json:"demo"`            // 试听音频是否已缓存（由 API 层填充）
}

// referenceDesigns 原创设计音色（design 模式）
var referenceDesigns = []ReferenceVoice{
	{Key: "design:warm-radio", Kind: "design", Name: "暖甜夜话", Sex: "女", Lang: "zh",
		Desc: "清亮甜润的年轻女声，像深夜电台主播，温柔治愈",
		Hint: "二十多岁的年轻女声，声音清亮甜润、气息柔和，像深夜电台的主播，温柔治愈，语速舒缓，句尾自然收住，亲切而不腻。"},
	{Key: "design:energetic-host", Kind: "design", Name: "元气好物官", Sex: "女", Lang: "zh",
		Desc: "明亮有元气的女声，语速轻快、情绪饱满，好物分享风",
		Hint: "二十多岁的年轻女声，声音明亮清脆、充满元气，语速轻快活泼，情绪饱满有感染力，像热情的购物分享博主，字字有精神。"},
	{Key: "design:poised-lady", Kind: "design", Name: "知性御姐", Sex: "女", Lang: "zh",
		Desc: "低沉优雅的成熟女声，从容有说服力",
		Hint: "三十岁左右的成熟女性声音，偏低沉优雅，气质知性从容，吐字清晰，语速平稳，有一种见过世面的说服力与分寸感。"},
	{Key: "design:soft-cute", Kind: "design", Name: "软萌小甜心", Sex: "女", Lang: "zh",
		Desc: "软糯甜美的年轻女声，尾音俏皮上扬",
		Hint: "二十岁出头的年轻女声，音色软糯甜美，气息轻柔，尾音自然上扬带一点俏皮，可爱但不做作，语速适中偏慢。"},
	{Key: "design:cool-fairy", Kind: "design", Name: "清冷仙气", Sex: "女", Lang: "zh",
		Desc: "清冷空灵的古风女声，淡雅疏离",
		Hint: "年轻女声，声线清冷空灵、干净纯粹，气息绵长，像古风配音与仙侠旁白，语速偏慢，淡雅而略带疏离感。"},
	{Key: "design:magnetic-bass", Kind: "design", Name: "磁性低音炮", Sex: "男", Lang: "zh",
		Desc: "低音浑厚有磁性的男声，纪录片旁白质感",
		Hint: "三十五岁左右的男声，低音浑厚有磁性，胸腔共鸣明显，沉稳大气，语速从容，像高质量纪录片与广告旁白。"},
	{Key: "design:sunny-boy", Kind: "design", Name: "阳光大男孩", Sex: "男", Lang: "zh",
		Desc: "明亮清爽有朝气的男声，运动博主风",
		Hint: "二十多岁的男声，明亮清爽、有朝气，语速轻快自然，情绪阳光积极，像户外运动与旅行 vlog 博主的口吻。"},
	{Key: "design:gentry-uncle", Kind: "design", Name: "儒雅说书人", Sex: "男", Lang: "zh",
		Desc: "温润儒雅的中年男声，娓娓道来",
		Hint: "四十岁左右的男声，温润儒雅、不急不躁，吐字讲究，语速从容，像读书人与说书人，娓娓道来有书卷气。"},
	{Key: "design:northern-buddy", Kind: "design", Name: "东北老铁", Sex: "男", Lang: "zh",
		Desc: "豪爽热情的东北男声，自带幽默感",
		Hint: "三十岁男声，东北口音底子，豪爽热情，嗓门敞亮，语气自带幽默感，像唠嗑一样自然，接地气不做作。"},
	{Key: "design:suspense-narrator", Kind: "design", Name: "悬疑解说", Sex: "男", Lang: "zh",
		Desc: "冷静克制的男声，带神秘感，解说节奏",
		Hint: "三十五岁左右的男声，冷静克制、略带神秘感，吐字有力，语速沉稳有节奏，句间留白，像悬疑推理类视频的解说。"},
}

// ReferenceVoices 返回完整参考音色目录（官方预置 + 原创设计）
func ReferenceVoices() []ReferenceVoice {
	descOf := map[string]string{
		"mimo_default": "官方默认音色，稳定百搭",
		"冰糖":          "官方女声，清亮甜美，最受欢迎",
		"茉莉":          "官方女声，温婉大方，叙述感强",
		"苏打":          "官方男声，干净清爽，日常口播",
		"白桦":          "官方男声，沉稳磁性，朗读向",
		"Mia":          "英文女声，自然流畅",
		"Chloe":        "英文女声，明亮活泼",
		"Milo":         "英文男声，沉稳清晰",
		"Dean":         "英文男声，低沉有质感",
	}
	sentenceOf := map[string]string{
		"mimo_default": "你好，我是清影的默认音色。粘贴链接，原画直出，文字变成声音也一样干脆。",
		"冰糖":           "嗨，我是冰糖。把链接交给我，无水印的原画视频马上还给你。",
		"茉莉":           "我是茉莉。一段文字，一点耐心，就能变成有温度的声音。",
		"苏打":           "我是苏打。喜欢的内容别只收藏，让声音陪你在路上继续听。",
		"白桦":           "我是白桦。读一段文字给你听，愿这个下午刚好安静。",
		"Mia":           "Hi, I'm Mia. Paste a link, and I will turn words into voice in seconds.",
		"Chloe":         "Hey there, I'm Chloe. Text in, voice out — that simple.",
		"Milo":          "This is Milo. Your words deserve a good voice.",
		"Dean":          "Dean here. Deep, clear, and ready to read for you.",
	}
	out := make([]ReferenceVoice, 0, len(PresetVoices)+len(referenceDesigns))
	for _, p := range PresetVoices {
		out = append(out, ReferenceVoice{
			Key: "preset:" + p.ID, Kind: "preset", Name: p.Name,
			Desc: descOf[p.ID], Sex: p.Sex, Lang: p.Lang, Voice: p.ID,
			Sentence: sentenceOf[p.ID],
		})
	}
	for _, d := range referenceDesigns {
		d.Sentence = "你好呀，我是「" + d.Name + "」。这是我的原创声音，希望你会喜欢——清影，让文字开口说话。"
		out = append(out, d)
	}
	return out
}

// FindReferenceVoice 按 key 查找参考音色
func FindReferenceVoice(key string) (ReferenceVoice, bool) {
	for _, v := range ReferenceVoices() {
		if v.Key == key {
			return v, true
		}
	}
	return ReferenceVoice{}, false
}

// Client MiMo 语音客户端
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	Preset  string
	Design  string
	Timeout time.Duration
	HTTP    *http.Client
}

// New 构造客户端
func New(baseURL, apiKey, model, preset, design string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		Preset:  preset,
		Design:  design,
		Timeout: timeout,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

// SynthRequest 合成参数
type SynthRequest struct {
	Mode       string
	Text       string
	Style      string
	Voice      string // 预置音色名或 data URI
	VoiceID    int64
	MaxChars   int
	Format     string
	VoiceClone string // 克隆样本 data URI
	DesignHint string
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type audioParam struct {
	Format string `json:"format"`
	Voice  string `json:"voice,omitempty"`
}

type chatBody struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Audio    audioParam    `json:"audio"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Audio struct {
				Data string `json:"data"`
			} `json:"audio"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Message string `json:"message"`
}

// Synthesize 合成语音，返回音频字节
func (c *Client) Synthesize(ctx context.Context, req SynthRequest) ([]byte, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, errors.New("尚未配置 MIMO API Key，请前往管理端「音色工坊」设置")
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, errors.New("合成文本不能为空")
	}
	maxChars := req.MaxChars
	if maxChars <= 0 {
		maxChars = 300
	}
	segments := SplitText(req.Text, maxChars)

	var parts [][]byte
	for i, seg := range segments {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := c.synthesizeOne(ctx, req, seg)
		if err != nil {
			if i == 0 {
				return nil, err
			}
			// 已有部分音频时保留成功片段，避免整体失败
			break
		}
		parts = append(parts, data)
	}
	if len(parts) == 0 {
		return nil, errors.New("上游未返回音频数据")
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return MergeWAV(parts), nil
}

func (c *Client) synthesizeOne(ctx context.Context, req SynthRequest, text string) ([]byte, error) {
	format := req.Format
	if format != "mp3" {
		format = "wav"
	}

	body := chatBody{Audio: audioParam{Format: format}}
	switch req.Mode {
	case ModePreset:
		body.Model = c.Preset
		if body.Model == "" {
			body.Model = "mimo-v2.5-tts"
		}
		body.Audio.Voice = req.Voice
		if body.Audio.Voice == "" {
			body.Audio.Voice = "冰糖"
		}
	case ModeDesign:
		body.Model = c.Design
		if body.Model == "" {
			body.Model = "mimo-v2.5-tts-voicedesign"
		}
		if req.DesignHint != "" {
			body.Messages = append(body.Messages, chatMessage{Role: "user", Content: req.DesignHint})
		}
	default: // clone
		body.Model = c.Model
		if body.Model == "" {
			body.Model = "mimo-v2.5-tts-voiceclone"
		}
		if strings.TrimSpace(req.VoiceClone) == "" {
			return nil, errors.New("克隆模式缺少参考音频")
		}
		body.Audio.Voice = req.VoiceClone
	}
	if req.Style != "" && req.Mode != ModeDesign {
		body.Messages = append(body.Messages, chatMessage{Role: "user", Content: req.Style})
	}
	body.Messages = append(body.Messages, chatMessage{Role: "assistant", Content: text})

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	url := c.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", c.APIKey)
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("语音服务响应超时，请缩短文本后重试")
		}
		return nil, fmt.Errorf("请求语音服务失败: %v", err)
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, errors.New("读取语音服务响应失败")
	}
	if resp.StatusCode >= 400 {
		var e chatResp
		_ = json.Unmarshal(payload, &e)
		msg := e.Message
		if e.Error != nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		if msg == "" {
			msg = strings.TrimSpace(string(payload))
			if len(msg) > 200 {
				msg = msg[:200]
			}
		}
		return nil, fmt.Errorf("语音服务返回 %d：%s", resp.StatusCode, msg)
	}

	var out chatResp
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, errors.New("语音服务返回了无法解析的内容")
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Audio.Data == "" {
		return nil, errors.New("语音服务未返回音频数据")
	}
	data, err := base64.StdEncoding.DecodeString(out.Choices[0].Message.Audio.Data)
	if err != nil {
		return nil, errors.New("音频数据解码失败")
	}
	return data, nil
}

// SplitText 按标点切分长文本，尽量保持语义完整
func SplitText(text string, maxChars int) []string {
	text = strings.TrimSpace(text)
	if maxChars <= 0 {
		maxChars = 300
	}
	if utf8.RuneCountInString(text) <= maxChars {
		return []string{text}
	}
	runes := []rune(text)
	var out []string
	start := 0
	for start < len(runes) {
		end := start + maxChars
		if end >= len(runes) {
			end = len(runes)
		} else {
			// 回退到最近的句末标点
			best := -1
			for i := end; i > start+maxChars/3; i-- {
				switch runes[i-1] {
				case '。', '！', '？', '；', '\n', '.', '!', '?', ';', '，', ',':
					best = i
				}
				if best > 0 {
					break
				}
			}
			if best > 0 {
				end = best
			}
		}
		seg := strings.TrimSpace(string(runes[start:end]))
		if seg != "" {
			out = append(out, seg)
		}
		start = end
	}
	return out
}

// MergeWAV 合并多段同格式 WAV（取第一段头，拼接 PCM 数据）
func MergeWAV(parts [][]byte) []byte {
	if len(parts) == 1 {
		return parts[0]
	}
	head := parts[0]
	dataOff, _, ok := wavDataChunk(head)
	if !ok {
		return parts[0]
	}
	header := make([]byte, dataOff)
	copy(header, head[:dataOff])
	var pcm []byte
	for _, p := range parts {
		off, n, ok := wavDataChunk(p)
		if !ok || off+n > len(p) {
			continue
		}
		pcm = append(pcm, p[off:off+n]...)
	}
	binary.LittleEndian.PutUint32(header[dataOff-4:], uint32(len(pcm)))
	if len(header) >= 8 {
		binary.LittleEndian.PutUint32(header[4:], uint32(len(pcm)+dataOff-8))
	}
	return append(header, pcm...)
}

// wavDataChunk 返回 data 块起始偏移与长度
func wavDataChunk(b []byte) (int, int, bool) {
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return 0, 0, false
	}
	pos := 12
	for pos+8 <= len(b) {
		id := string(b[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
		if id == "data" {
			if pos+8+size > len(b) {
				size = len(b) - pos - 8
			}
			return pos + 8, size, true
		}
		pos += 8 + size + (size & 1)
	}
	return 0, 0, false
}

// WAVDurationMs 解析 WAV 时长（毫秒）
func WAVDurationMs(b []byte) int {
	if len(b) < 44 || string(b[0:4]) != "RIFF" {
		return 0
	}
	byteRate := int(binary.LittleEndian.Uint32(b[28:32]))
	_, n, ok := wavDataChunk(b)
	if !ok || byteRate <= 0 {
		return 0
	}
	return int(int64(n) * 1000 / int64(byteRate))
}

// EstimateDurationMs 估算时长：WAV 精确解析，mp3 按 128kbps 估算
func EstimateDurationMs(b []byte, mime string) int {
	if ms := WAVDurationMs(b); ms > 0 {
		return ms
	}
	if strings.Contains(mime, "mpeg") || strings.Contains(mime, "mp3") {
		return int(int64(len(b)) * 8 * 1000 / 128000)
	}
	return 0
}

// DetectMime 依据文件头判断音频类型
func DetectMime(b []byte) string {
	if len(b) > 12 {
		if string(b[0:4]) == "RIFF" && string(b[8:12]) == "WAVE" {
			return "audio/wav"
		}
		if string(b[0:3]) == "ID3" {
			return "audio/mpeg"
		}
		if b[0] == 0xFF && (b[1]&0xE0) == 0xE0 {
			return "audio/mpeg"
		}
		if string(b[4:8]) == "ftyp" {
			return "audio/mp4"
		}
		if string(b[0:4]) == "OggS" {
			return "audio/ogg"
		}
	}
	return ""
}

// DataURI 组装 data URI
func DataURI(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}
