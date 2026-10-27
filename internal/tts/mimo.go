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
