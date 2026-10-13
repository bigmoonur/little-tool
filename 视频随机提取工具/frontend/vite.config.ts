import {defineConfig, type Plugin} from 'vite'
import react from '@vitejs/plugin-react'

// 缩略图和生成缩略图用的视频数据由 Go 的 AssetServer.Handler 提供。
// 开发时 Vite 会给找不到的地址回 index.html，这两个前缀要回 404，Wails 才会把请求交给 Go。打包后没有 Vite，不受影响。
const goRoutes = (): Plugin => ({
  name: 'go-routes',
  configureServer(server) {
    server.middlewares.use((req, res, next) => {
      if (!req.url?.startsWith('/media/') && !req.url?.startsWith('/thumbs/')) return next()
      res.statusCode = 404
      res.end()
    })
  },
})

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react(), goRoutes()]
})
