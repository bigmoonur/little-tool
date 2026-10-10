import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import '@fontsource/barlow-condensed/latin-500.css'
import '@fontsource/barlow-condensed/latin-600.css'
import './styles.css'
import App from './App'

// 桌面程序里屏蔽浏览器默认右键菜单（输入框保留，方便复制粘贴）。
document.addEventListener('contextmenu', e => {
  const t = e.target as HTMLElement
  if (!t.closest('input, textarea, .result-lines')) e.preventDefault()
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
