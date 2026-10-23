import { defineConfig, externalizeDepsPlugin } from 'electron-vite'

// Структура по умолчанию electron-vite:
//   src/main/index.ts, src/preload/index.ts, src/renderer/index.html.
// Явные input'ы не задаём, чтобы не дублировать соглашение фреймворка.
export default defineConfig({
  main: {
    plugins: [externalizeDepsPlugin()],
  },
  preload: {
    plugins: [externalizeDepsPlugin()],
  },
  renderer: {},
})
