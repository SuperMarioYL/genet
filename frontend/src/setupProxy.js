const { createProxyMiddleware } = require('http-proxy-middleware');

module.exports = function(app) {
  // API 请求直接代理到后端
  app.use(
    '/api',
    createProxyMiddleware({
      target: 'http://localhost:8080',
      changeOrigin: true,
    })
  );
};

