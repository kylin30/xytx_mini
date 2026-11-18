// 填你的云托管环境 ID 和服务名（云托管控制台可以看到）
const CLOUD_ENV = '';
const CLOUD_SERVICE = '';

App({
  onLaunch() {
    if (!wx.cloud) {
      console.error('基础库版本过低，云托管需要 2.23.0 及以上');
      return;
    }
    wx.cloud.init({ env: CLOUD_ENV });
  },

  // 调云托管服务；业务失败（code != 0）时抛出带服务端原文的错误
  async call(path, data) {
    if (!CLOUD_ENV || !CLOUD_SERVICE) {
      throw new Error('先在 app.js 里填 CLOUD_ENV 和 CLOUD_SERVICE');
    }
    const res = await wx.cloud.callContainer({
      config: { env: CLOUD_ENV },
      path,
      method: 'POST',
      header: { 'X-WX-SERVICE': CLOUD_SERVICE, 'content-type': 'application/json' },
      data,
    });
    const body = res.data || {};
    if (body.code !== 0) {
      const err = new Error(body.errorMsg || ('HTTP ' + res.statusCode));
      err.data = body.data;
      throw err;
    }
    return body.data;
  },
});
