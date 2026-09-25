module.exports = {
  devServer: {
    port: 8080,
    proxy: {
      '/api': {
        target: process.env.COGNIGO_API_URL || 'http://localhost:9090',
        changeOrigin: true,
        pathRewrite: {
          '^/api': '/api/v1'
        }
      }
    }
  }
}
