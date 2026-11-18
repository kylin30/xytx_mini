const app = getApp();

Page({
  data: {
    state: '',
    phase: 'loading', // loading | ready | done | expired
    busy: false,
    error: '',
    phoneMasked: '',
  },

  onLoad(options) {
    // 小程序码的 scene 会经过 URL 编码放在 options.scene 里
    const state = decodeURIComponent(options.scene || '');
    this.setData({ state });
    if (state) this.reportScanned(state);
  },

  async reportScanned(state) {
    try {
      const v = await app.call('/api/qrlogin/scan', { state });
      this.applyStatus(v);
    } catch (e) {
      this.setData({ phase: 'ready', error: '报告扫码失败：' + e.message });
    }
  },

  applyStatus(v) {
    if (!v) return;
    if (v.status === 'done') this.setData({ phase: 'done', phoneMasked: v.phoneMasked });
    else if (v.status === 'expired') this.setData({ phase: 'expired' });
    else this.setData({ phase: 'ready' });
  },

  async onGetPhoneNumber(e) {
    const { code, errMsg, errno } = e.detail;
    if (!code) {
      // 用户拒绝、未在隐私保护指引里声明手机号、额度用完等都会走到这里
      this.setData({ error: '没有拿到 code：' + errMsg + (errno ? '（errno ' + errno + '）' : '') });
      return;
    }
    this.setData({ busy: true, error: '' });
    try {
      const v = await app.call('/api/qrlogin/phone', { state: this.data.state, code });
      this.applyStatus(v);
    } catch (err) {
      this.setData({ error: err.message });
      this.applyStatus(err.data);
    } finally {
      this.setData({ busy: false });
    }
  },
});
