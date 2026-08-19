// 后端服务地址（替换成你自己的 HTTPS 域名）
// 注意：必须是 HTTPS 且在微信公众平台配置了 uploadFile 合法域名
const BASE_URL = 'https://your-domain.com';

Page({
  data: {
    src: '',
    result: '',
    size: 12,
    colors: 32,
    loading: false
  },

  // 选择图片
  chooseImage() {
    wx.chooseMedia({
      count: 1,
      mediaType: ['image'],
      success: (res) => {
        this.setData({ src: res.tempFiles[0].tempFilePath, result: '' });
      }
    });
  },

  onSizeChange(e) {
    this.setData({ size: e.detail.value });
  },

  onColorsChange(e) {
    this.setData({ colors: e.detail.value });
  },

  // 上传并转换
  convert() {
    if (!this.data.src) {
      wx.showToast({ title: '请先选择图片', icon: 'none' });
      return;
    }

    this.setData({ loading: true });

    wx.uploadFile({
      url: BASE_URL + '/api/pixelate-json',
      filePath: this.data.src,
      name: 'image',
      formData: {
        size: String(this.data.size),
        colors: String(this.data.colors)
      },
      success: (res) => {
        // wx.uploadFile 返回的 res.data 是字符串，需要 JSON.parse
        let data;
        try {
          data = JSON.parse(res.data);
        } catch (e) {
          wx.showToast({ title: '返回数据解析失败', icon: 'none' });
          return;
        }

        if (data.code === 0) {
          this.setData({ result: BASE_URL + data.url });
        } else {
          wx.showToast({ title: data.msg || '生成失败', icon: 'none' });
        }
      },
      fail: (err) => {
        wx.showToast({ title: '请求失败：' + err.errMsg, icon: 'none' });
      },
      complete: () => {
        this.setData({ loading: false });
      }
    });
  }
});
