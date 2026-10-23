import type { DesktopApi } from '../../shared/ipc'

declare global {
  interface Window {
    asledger: DesktopApi
  }
}

export {}
