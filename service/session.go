package service

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"

	"wxcloudrun-golang/db"
)

// 会话存在云托管 MySQL 的 qrlogin_session 表里，多个实例共用，实例数不用限制。
const (
	sessionTTLSeconds = 300
	maxSessions       = 200

	statusWaiting = "waiting" // 已出码，等待扫码
	statusScanned = "scanned" // 小程序已打开，等待用户点按钮
	statusDone    = "done"    // 已拿到手机号
	statusExpired = "expired" // 超时或不存在（不入库，查不到就是它）
)

// newState 生成 32 位十六进制会话号，正好是小程序码 scene 的长度上限
func newState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// addSession 新建会话；返回 false 表示同时存在的会话太多
func addSession(state, envVersion string) (bool, error) {
	cli := db.Get()
	// 顺手清掉过期的
	if _, err := cli.Exec("DELETE FROM qrlogin_session WHERE expires_at < UTC_TIMESTAMP(3) LIMIT 1000"); err != nil {
		return false, err
	}
	var live int
	if err := cli.QueryRow("SELECT COUNT(*) FROM qrlogin_session WHERE expires_at > UTC_TIMESTAMP(3)").Scan(&live); err != nil {
		return false, err
	}
	if live >= maxSessions {
		return false, nil
	}
	_, err := cli.Exec(`INSERT INTO qrlogin_session (state, status, env_version, created_at, expires_at)
		VALUES (?, ?, ?, UTC_TIMESTAMP(3), DATE_ADD(UTC_TIMESTAMP(3), INTERVAL ? SECOND))`,
		state, statusWaiting, envVersion, sessionTTLSeconds)
	return err == nil, err
}

// getView 读会话；不存在或已过期返回 expired
func getView(state string) (statusView, error) {
	v := statusView{State: state}
	var scan, done sql.NullFloat64
	err := db.Get().QueryRow(`SELECT status, env_version,
			GREATEST(TIMESTAMPDIFF(SECOND, UTC_TIMESTAMP(3), expires_at), 0),
			TIMESTAMPDIFF(MICROSECOND, created_at, scanned_at) / 1e6,
			TIMESTAMPDIFF(MICROSECOND, created_at, done_at) / 1e6,
			phone_masked, country_code, openid_tail, last_error
		FROM qrlogin_session WHERE state = ? AND expires_at > UTC_TIMESTAMP(3)`, state).
		Scan(&v.Status, &v.EnvVersion, &v.ExpiresInSeconds, &scan, &done,
			&v.PhoneMasked, &v.CountryCode, &v.OpenIDTail, &v.LastError)
	if err == sql.ErrNoRows {
		return statusView{State: state, Status: statusExpired}, nil
	}
	if err != nil {
		return v, err
	}
	v.ScanSeconds = scan.Float64
	v.DoneSeconds = done.Float64
	return v, nil
}

// markScanned 只把 waiting 改成 scanned，其他状态不动
func markScanned(state, openIDTail string) error {
	_, err := db.Get().Exec(`UPDATE qrlogin_session SET status = ?, scanned_at = UTC_TIMESTAMP(3), openid_tail = ?
		WHERE state = ? AND status = ? AND expires_at > UTC_TIMESTAMP(3)`,
		statusScanned, openIDTail, state, statusWaiting)
	return err
}

func recordError(state, msg string) error {
	if len(msg) > 500 {
		msg = msg[:500]
	}
	_, err := db.Get().Exec(`UPDATE qrlogin_session SET last_error = ?
		WHERE state = ? AND status <> ? AND expires_at > UTC_TIMESTAMP(3)`, msg, state, statusDone)
	return err
}

// markDone 条件更新保证一个会话只完成一次（多个实例同时提交时只有一个生效）
func markDone(state, phoneMasked, countryCode, openIDTail string) error {
	_, err := db.Get().Exec(`UPDATE qrlogin_session SET status = ?, done_at = UTC_TIMESTAMP(3),
			scanned_at = COALESCE(scanned_at, UTC_TIMESTAMP(3)),
			phone_masked = ?, country_code = ?, openid_tail = ?, last_error = ''
		WHERE state = ? AND status <> ? AND expires_at > UTC_TIMESTAMP(3)`,
		statusDone, phoneMasked, countryCode, openIDTail, state, statusDone)
	return err
}

func maskPhone(p string) string {
	if len(p) < 7 {
		return "****"
	}
	return p[:3] + "****" + p[len(p)-4:]
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
