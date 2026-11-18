package main

import (
	"fmt"
	"log"
	"net/http"

	"wxcloudrun-golang/db"
	"wxcloudrun-golang/service"
)

// 扫码 + 小程序手机号快速验证 的联调测试服务（替换了模板原来的计数器）
func main() {
	if err := db.Init(); err != nil {
		panic(fmt.Sprintf("mysql init failed with %+v", err))
	}

	http.HandleFunc("/", service.IndexHandler)
	// 网页端（模拟 PC / 网页客户端）：出码、轮询结果
	http.HandleFunc("/api/qrlogin/create", service.CreateHandler)
	http.HandleFunc("/api/qrlogin/status", service.StatusHandler)
	// 小程序端（wx.cloud.callContainer 调用）：报告已扫码、提交手机号 code
	http.HandleFunc("/api/qrlogin/scan", service.ScanHandler)
	http.HandleFunc("/api/qrlogin/phone", service.PhoneHandler)

	log.Fatal(http.ListenAndServe(":80", nil))
}
