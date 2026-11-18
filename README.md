# xytx_mini：扫码手机号登录联调测试

基于微信云托管 golang 模板，把计数器换成「电脑出码 → 手机微信扫码 → 小程序手机号快速验证 → 电脑拿到手机号」的联调测试。只用于验证能力，不是正式服务。会话存在云托管 MySQL 的 `qrlogin_session` 表里，多个实例共用，实例数不用限制。

## 组成

| 部分 | 位置 | 作用 |
| --- | --- | --- |
| 网页测试页 | `index.html`（服务根路径 `/`） | 模拟 PC / 网页客户端：选小程序版本、生成小程序码、轮询结果 |
| 服务端 | `main.go`、`service/`、`db/` | 出码（`getwxacodeunlimit`）、用 code 换手机号（`getuserphonenumber`），都走云托管「开放接口服务」，不需要 AppSecret |
| 小程序测试工程 | `miniprogram-test/` | 一个页面 `pages/qrlogin/index`：读 scene 里的会话号，点「手机号快速验证」后把 code 交给服务端（不打进镜像） |

## 接口

| 接口 | 调用方 | 说明 |
| --- | --- | --- |
| `POST /api/qrlogin/create` `{envVersion, page}` | 网页 | 生成 32 位会话号和带它的小程序码（data URL），5 分钟有效 |
| `GET /api/qrlogin/status?state=` | 网页 | 状态 waiting / scanned / done / expired，完成后带打码手机号 |
| `POST /api/qrlogin/scan` `{state}` | 小程序 | 报告已扫码 |
| `POST /api/qrlogin/phone` `{state, code}` | 小程序 | 用 code 换手机号；完成或过期的会话不再接受 |

小程序的两个接口只接受 `wx.cloud.callContainer` 发来的请求（要有 `X-WX-OPENID`）。手机号在所有返回、日志和数据库里都是打码的（`138****8000`）。

会话的时间一律用数据库的 `UTC_TIMESTAMP(3)` 写和比较，不受各实例时钟影响；完成用条件更新，同一个会话只会完成一次；每次出码时顺手删掉过期的行。

## 测试步骤

1. **云托管部署本服务**，然后在控制台：
   - 开通云托管 MySQL，服务的环境变量里要有 `MYSQL_ADDRESS`、`MYSQL_USERNAME`、`MYSQL_PASSWORD`（模板部署时会自动填）；库名用 `MYSQL_DATABASE`，不填默认 `xytx_mini`。库和表在服务启动时自动创建，连不上数据库服务会启动失败；
   - 打开**公网访问**，记下默认域名；
   - 「云调用 → 微信令牌权限配置」加上 `/wxa/getwxacodeunlimit` 和 `/wxa/business/getuserphonenumber`，不加会报 `access_token missing`。
2. **小程序后台**：「设置 → 服务内容声明 → 用户隐私保护指引」里声明收集手机号，否则按钮回调直接失败。
3. **小程序工程**：用微信开发者工具导入 `miniprogram-test`，AppID 填企业小程序的；在 `miniprogram/app.js` 填 `CLOUD_ENV`（云托管环境 ID）和 `CLOUD_SERVICE`（服务名）。
4. 两种测法：
   - **真扫码**：开发者工具「上传」成开发版本（不影响线上版）。浏览器打开云托管公网域名，选「开发版」，生成二维码，用项目成员的微信扫码，点「手机号快速验证」，网页上应显示打码手机号和用时。
   - **不上传**：网页上生成二维码后复制会话号，开发者工具编译模式「扫码登录测试」把启动参数改成 `scene=会话号`，再点「预览」用手机扫开发者工具给的预览码。

## 费用

每次按钮回调成功扣一次（0.03 元/次，2023 年公告价，以小程序后台「付费管理」为准）；每个小程序有 1000 次体验额度，开发版、体验版、正式版共用。服务端换号失败不会另外扣费。

## 本地联调

`WX_API_BASE` 可以把微信接口指到模拟服务，只用于本地联调，线上不要设置。
