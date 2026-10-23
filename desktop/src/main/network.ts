import { networkInterfaces } from 'node:os'
import type { NetworkAddress } from '../shared/ipc'

export interface NetworkInterfaceAddress {
  address: string
  family: string
  internal: boolean
}

export type NetworkInterfaceMap = Record<
  string,
  NetworkInterfaceAddress[] | undefined
>

/** Отбрасывает loopback, link-local, multicast и зарезервированные IPv4. */
export function isUsableIPv4(address: string): boolean {
  const parts = address.split('.')
  if (parts.length !== 4) {
    return false
  }
  const octets = parts.map((part) => Number(part))
  if (
    octets.some(
      (octet) => !Number.isInteger(octet) || octet < 0 || octet > 255,
    )
  ) {
    return false
  }

  const first = octets[0] ?? 0
  const second = octets[1] ?? 0
  if (first === 0 || first === 127 || first >= 224) {
    return false
  }
  if (first === 169 && second === 254) {
    return false
  }
  return true
}

function addressKey(address: string): number {
  return address
    .split('.')
    .reduce((result, part) => result * 256 + Number(part), 0)
}

function interfaceRank(name: string): number {
  const lower = name.toLowerCase()
  if (/^(wl|wlan|wi-?fi)/.test(lower)) {
    return 0
  }
  if (/^(en|eth)/.test(lower)) {
    return 1
  }
  if (
    /^(vethernet|virtual|vmnet|vbox|docker|br-|veth|virbr|tun|tap|wg|tailscale|zt)/.test(
      lower,
    )
  ) {
    return 3
  }
  return 2
}

/**
 * Собирает IPv4-адреса, по которым телефон в той же сети может достучаться
 * до web и API. Одинаковые адреса на разных интерфейсах схлопываются.
 */
export function listLocalIPv4(
  interfaces: NetworkInterfaceMap,
  webPort: number,
  apiPort: number,
): NetworkAddress[] {
  const seen = new Set<string>()
  const result: NetworkAddress[] = []

  for (const [interfaceName, addresses] of Object.entries(interfaces)) {
    if (!addresses) {
      continue
    }
    for (const info of addresses) {
      if (info.internal || info.family !== 'IPv4') {
        continue
      }
      if (!isUsableIPv4(info.address) || seen.has(info.address)) {
        continue
      }
      seen.add(info.address)
      result.push({
        interfaceName,
        address: info.address,
        apiUrl: `http://${info.address}:${apiPort}`,
        webUrl: `http://${info.address}:${webPort}`,
      })
    }
  }

  return result.sort((left, right) => {
    const byRank =
      interfaceRank(left.interfaceName) - interfaceRank(right.interfaceName)
    if (byRank !== 0) {
      return byRank
    }
    const byAddress = addressKey(left.address) - addressKey(right.address)
    return byAddress !== 0
      ? byAddress
      : left.interfaceName.localeCompare(right.interfaceName)
  })
}

/** Читает актуальные интерфейсы через os.networkInterfaces(). */
export function currentLocalIPv4(
  webPort: number,
  apiPort: number,
): NetworkAddress[] {
  return listLocalIPv4(networkInterfaces(), webPort, apiPort)
}

/** Периодически проверяет смену Wi-Fi/Ethernet/VPN и сообщает об изменениях. */
export class NetworkMonitor {
  private timer: ReturnType<typeof setInterval> | null = null
  private lastKey = ''

  constructor(
    private readonly getAddresses: () => NetworkAddress[],
    private readonly onChange: (addresses: NetworkAddress[]) => void,
    private readonly intervalMs = 5000,
  ) {}

  start(): void {
    if (this.timer) {
      return
    }
    this.refresh()
    this.timer = setInterval(() => {
      this.refresh()
    }, this.intervalMs)
    const timer = this.timer as ReturnType<typeof setInterval> & {
      unref?: () => void
    }
    timer.unref?.()
  }

  stop(): void {
    if (this.timer) {
      clearInterval(this.timer)
      this.timer = null
    }
  }

  refresh(): NetworkAddress[] {
    const addresses = this.getAddresses()
    const key = JSON.stringify(addresses)
    if (key !== this.lastKey) {
      this.lastKey = key
      this.onChange(addresses)
    }
    return addresses
  }
}
