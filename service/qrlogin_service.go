package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"time"
)

// JsonResult 返回结构（沿用模板）
type JsonResult struct {
	Code     int         `json:"code"`
	ErrorMsg string      `json:"errorMsg,omitempty"`
	Data     interface{} `json:"data,omitempty"`
}

var (
	stateRe = regexp.MustCompile(`^[0-9a-f]{32}$`)
	pageRe  = regexp.MustCompile(`^[A-Za-z0-9_\-]+(/[A-Za-z0-9_\-]+)*$`)
)

// IndexHandler 网页端测试页
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, err := ioutil.ReadFile("./index.html")
	if err != nil {
		http.Error(w, "内部错误", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}

func writeJSON(w http.ResponseWriter, status int, res *JsonResult) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(res)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, &JsonResult{Code: -1, ErrorMsg: msg})
}

func readBody(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, fmt.Sprintf("请求方法 %s 不支持", r.Method))
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		fail(w, http.StatusBadRequest, "请求体不是合法的 JSON")
		return false
	}
	return true
}

type statusView struct {
	State            string  `json:"state"`
	Status           string  `json:"status"`
	EnvVersion       string  `json:"envVersion,omitempty"`
	ExpiresInSeconds int     `json:"expiresInSeconds"`
	ScanSeconds      float64 `json:"scanSeconds,omitempty"`
	DoneSeconds      float64 `json:"doneSeconds,omitempty"`
	PhoneMasked      string  `json:"phoneMasked,omitempty"`
	CountryCode      string  `json:"countryCode,omitempty"`
	OpenIDTail       string  `json:"openIdTail,omitempty"`
	LastError        string  `json:"lastError,omitempty"`
}

// CreateHandler 网页端：新建会话并生成带会话号的小程序码
func CreateHandler(w http.ResponseWriter, r *http.Request) {
	var in struct {
		EnvVersion string `json:"envVersion"`
		Page       string `json:"page"`
	}
	if !readBody(w, r, &in) {
		return
	}
	switch in.EnvVersion {
	case "release", "trial", "develop":
	default:
		fail(w, http.StatusBadRequest, "envVersion 只能是 release、trial、develop")
		return
	}
	if !pageRe.MatchString(in.Page) || len(in.Page) > 128 {
		fail(w, http.StatusBadRequest, "page 格式不对：填小程序页面路径，开头不带 /，例如 pages/qrlogin/index")
		return
	}
	state, err := newState()
	if err != nil {
		fail(w, http.StatusInternalServerError, "生成会话号失败")
		return
	}
	img, contentType, err := getUnlimitedQRCode(state, in.Page, in.EnvVersion)
	if err != nil {
		log.Printf("create %s: 生成小程序码失败: %v", state[:8], err)
		fail(w, http.StatusBadGateway, "生成小程序码失败："+err.Error())
		return
	}
	added, err := addSession(state, in.EnvVersion)
	if err != nil {
		log.Printf("create %s: 写会话失败: %v", state[:8], err)
		fail(w, http.StatusInternalServerError, "写会话失败")
		return
	}
	if !added {
		fail(w, http.StatusTooManyRequests, "同时进行的测试会话太多，请稍后再试")
		return
	}
	log.Printf("create %s: env=%s page=%s", state[:8], in.EnvVersion, in.Page)
	writeJSON(w, http.StatusOK, &JsonResult{Data: map[string]string{
		"state":  state,
		"qrcode": "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(img),
	}})
}

// StatusHandler 网页端：轮询会话状态
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if !stateRe.MatchString(state) {
		fail(w, http.StatusBadRequest, "state 格式不对")
		return
	}
	v, err := getView(state)
	if err != nil {
		log.Printf("status %s: 读会话失败: %v", state[:8], err)
		fail(w, http.StatusInternalServerError, "读会话失败")
		return
	}
	writeJSON(w, http.StatusOK, &JsonResult{Data: v})
}

// 小程序经 wx.cloud.callContainer 调用时，云托管网关会带上 X-WX-OPENID
func miniProgramOpenID(w http.ResponseWriter, r *http.Request) (string, bool) {
	openID := r.Header.Get("X-WX-OPENID")
	if openID == "" {
		fail(w, http.StatusForbidden, "只接受小程序通过 wx.cloud.callContainer 发来的请求")
		return "", false
	}
	return openID, true
}

// ScanHandler 小程序：打开页面后报告已扫码
func ScanHandler(w http.ResponseWriter, r *http.Request) {
	var in struct {
		State string `json:"state"`
	}
	if !readBody(w, r, &in) {
		return
	}
	openID, ok := miniProgramOpenID(w, r)
	if !ok {
		return
	}
	if !stateRe.MatchString(in.State) {
		fail(w, http.StatusBadRequest, "会话号格式不对")
		return
	}
	if err := markScanned(in.State, tail(openID, 6)); err != nil {
		log.Printf("scan %s: 写会话失败: %v", in.State[:8], err)
		fail(w, http.StatusInternalServerError, "写会话失败")
		return
	}
	v, err := getView(in.State)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读会话失败")
		return
	}
	log.Printf("scan %s: status=%s", in.State[:8], v.Status)
	writeJSON(w, http.StatusOK, &JsonResult{Data: v})
}

// PhoneHandler 小程序：提交手机号快速验证组件给的 code，服务端换出手机号
func PhoneHandler(w http.ResponseWriter, r *http.Request) {
	var in struct {
		State string `json:"state"`
		Code  string `json:"code"`
	}
	if !readBody(w, r, &in) {
		return
	}
	openID, ok := miniProgramOpenID(w, r)
	if !ok {
		return
	}
	if !stateRe.MatchString(in.State) {
		fail(w, http.StatusBadRequest, "会话号格式不对")
		return
	}
	if in.Code == "" || len(in.Code) > 256 {
		fail(w, http.StatusBadRequest, "缺少 code")
		return
	}

	// 先确认会话还能用，避免过期会话白白消耗 code
	current, err := getView(in.State)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读会话失败")
		return
	}
	if current.Status == statusDone || current.Status == statusExpired {
		writeJSON(w, http.StatusConflict, &JsonResult{Code: -1, ErrorMsg: "这个二维码已失效或已完成，请在网页上重新生成", Data: current})
		return
	}

	started := time.Now()
	info, err := getUserPhoneNumber(in.Code)
	cost := time.Since(started)
	if err != nil {
		msg := err.Error()
		var we *wxError
		if errors.As(err, &we) {
			msg = fmt.Sprintf("换手机号失败：errcode=%d，%s", we.ErrCode, we.ErrMsg)
		}
		log.Printf("phone %s: 失败（%v）: %s", in.State[:8], cost, msg)
		if err := recordError(in.State, msg); err != nil {
			log.Printf("phone %s: 记录错误失败: %v", in.State[:8], err)
		}
		fail(w, http.StatusBadGateway, msg)
		return
	}

	if err := markDone(in.State, maskPhone(info.PurePhoneNumber), info.CountryCode, tail(openID, 6)); err != nil {
		log.Printf("phone %s: 写会话失败: %v", in.State[:8], err)
		fail(w, http.StatusInternalServerError, "写会话失败")
		return
	}
	v, err := getView(in.State)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读会话失败")
		return
	}
	// 日志里不记手机号
	log.Printf("phone %s: 成功（微信接口耗时 %v），status=%s", in.State[:8], cost, v.Status)
	writeJSON(w, http.StatusOK, &JsonResult{Data: v})
}
