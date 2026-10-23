import { defineConfig, externalizeDepsPlugin } from 'electron-vite'

// Структура по умолчанию electron-vite:
//   src/main/index.ts, src/preload/index.ts, src/renderer/index.html.
// Явные input'ы не задаём, чтобы не дублировать соглашение фреймворка.
export default defineConfig({
  main: {
    // @noble/hashes — ESM-зависимость для Argon2id в main; включаем её в
    // бандл, чтобы не зависеть от require(esm) и состава node_modules.
    plugins: [externalizeDepsPlugin({ exclude: ['@noble/hashes'] })],
  },
  preload: {
    plugins: [externalizeDepsPlugin()],
  },
  renderer: {},
})
