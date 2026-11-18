package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// 走微信云托管「开放接口服务」（云调用）：容器内用 http 访问 api.weixin.qq.com，不带 access_token，
// 由云托管代为鉴权。需要在云托管控制台「云调用 → 微信令牌权限配置」里加上用到的两个接口路径。
// WX_API_BASE 只给本地联调指向模拟接口用，线上不要设置。
var wxAPIBase = func() string {
	if v := strings.TrimSpace(os.Getenv("WX_API_BASE")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://api.weixin.qq.com"
}()

var wxClient = &http.Client{Timeout: 10 * time.Second}

type wxError struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (e *wxError) Error() string {
	return fmt.Sprintf("微信接口返回 errcode=%d，errmsg=%s", e.ErrCode, e.ErrMsg)
}

func postWx(path string, body interface{}) (*http.Response, []byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	resp, err := wxClient.Post(wxAPIBase+path, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("调用微信接口 %s 失败：%v", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("读取微信接口 %s 的返回失败：%v", path, err)
	}
	return resp, data, nil
}

// getUnlimitedQRCode 生成小程序码（不限数量），scene 最多 32 个字符。
// 成功时微信直接返回图片；失败时返回 JSON 错误。
func getUnlimitedQRCode(scene, page, envVersion string) ([]byte, string, error) {
	resp, data, err := postWx("/wxa/getwxacodeunlimit", map[string]interface{}{
		"scene":       scene,
		"page":        page,
		"check_path":  false, // 页面还没发布到正式版时必须为 false
		"env_version": envVersion,
		"width":       280,
	})
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "image/") {
		return data, contentType, nil
	}
	var we wxError
	if json.Unmarshal(data, &we) == nil && we.ErrCode != 0 {
		return nil, "", &we
	}
	return nil, "", fmt.Errorf("生成小程序码返回了无法识别的内容（HTTP %d，Content-Type=%s）", resp.StatusCode, contentType)
}

type wxPhoneInfo struct {
	PhoneNumber     string `json:"phoneNumber"`
	PurePhoneNumber string `json:"purePhoneNumber"`
	CountryCode     string `json:"countryCode"`
	Watermark       struct {
		Timestamp int64  `json:"timestamp"`
		AppID     string `json:"appid"`
	} `json:"watermark"`
}

// getUserPhoneNumber 用小程序 getPhoneNumber 回调里的 code 换手机号（code 5 分钟内有效，只能用一次）。
func getUserPhoneNumber(code string) (*wxPhoneInfo, error) {
	_, data, err := postWx("/wxa/business/getuserphonenumber", map[string]string{"code": code})
	if err != nil {
		return nil, err
	}
	var res struct {
		wxError
		PhoneInfo *wxPhoneInfo `json:"phone_info"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf("换手机号返回的内容不是 JSON：%v", err)
	}
	if res.ErrCode != 0 {
		return nil, &res.wxError
	}
	if res.PhoneInfo == nil || res.PhoneInfo.PurePhoneNumber == "" {
		return nil, fmt.Errorf("换手机号成功但没有返回号码")
	}
	return res.PhoneInfo, nil
}
