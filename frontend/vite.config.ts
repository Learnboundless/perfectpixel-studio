import {readFile} from 'fs/promises'
import {resolve} from 'path'
import {defineConfig, type Plugin} from 'vite'
import react from '@vitejs/plugin-react'

function serveRootIndexForWails(): Plugin {
  return {
    name: 'perfectpixel-wails-root-index',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        const url = req.url ?? ''
        if (req.method !== 'GET' || (url !== '/' && !url.startsWith('/?'))) {
          next()
          return
        }
        try {
          const html = await readFile(resolve(server.config.root, 'index.html'), 'utf8')
          const transformed = await server.transformIndexHtml(url, html)
          res.statusCode = 200
          res.setHeader('Content-Type', 'text/html; charset=utf-8')
          res.end(transformed)
        } catch (err) {
          next(err)
        }
      })
    },
  }
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [serveRootIndexForWails(), react()]
})
