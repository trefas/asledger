import { afterEach, describe, expect, it, vi } from 'vitest'
import type { NetworkAddress } from '../src/shared/ipc'
import {
  NetworkMonitor,
  isUsableIPv4,
  listLocalIPv4,
  type NetworkInterfaceMap,
} from '../src/main/network'

const interfaces: NetworkInterfaceMap = {
  lo: [{ address: '127.0.0.1', family: 'IPv4', internal: true }],
  wlan0: [
    { address: '192.168.1.5', family: 'IPv4', internal: false },
    { address: 'fe80::1', family: 'IPv6', internal: false },
    { address: '169.254.10.1', family: 'IPv4', internal: false },
  ],
  eth0: [{ address: '10.0.0.2', family: 'IPv4', internal: false }],
  vpn0: [{ address: '100.100.1.1', family: 'IPv4', internal: false }],
  docker0: [{ address: '172.17.0.1', family: 'IPv4', internal: false }],
  duplicate: [{ address: '192.168.1.5', family: 'IPv4', internal: false }],
}

describe('локальные IPv4', () => {
  it('оставляет несколько адаптеров и VPN, убирая loopback и link-local', () => {
    const result = listLocalIPv4(interfaces, 8777, 8778)

    expect(result.map((item) => item.address)).toEqual([
      '192.168.1.5',
      '10.0.0.2',
      '100.100.1.1',
      '172.17.0.1',
    ])
    expect(result.find((item) => item.address === '192.168.1.5')).toMatchObject(
      {
        interfaceName: 'wlan0',
        apiUrl: 'http://192.168.1.5:8778',
        webUrl: 'http://192.168.1.5:8777',
      },
    )
  })

  it('возвращает пустой список без сети', () => {
    expect(listLocalIPv4({}, 8777, 8778)).toEqual([])
  })

  it('распознаёт служебные IPv4', () => {
    expect(isUsableIPv4('192.168.1.5')).toBe(true)
    expect(isUsableIPv4('10.0.0.2')).toBe(true)
    expect(isUsableIPv4('100.100.1.1')).toBe(true)
    expect(isUsableIPv4('127.0.0.1')).toBe(false)
    expect(isUsableIPv4('169.254.1.1')).toBe(false)
    expect(isUsableIPv4('0.0.0.0')).toBe(false)
    expect(isUsableIPv4('224.0.0.1')).toBe(false)
    expect(isUsableIPv4('255.255.255.255')).toBe(false)
    expect(isUsableIPv4('999.1.1.1')).toBe(false)
    expect(isUsableIPv4('1.2.3')).toBe(false)
    expect(isUsableIPv4('abc')).toBe(false)
  })
})

describe('NetworkMonitor', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('сообщает только об изменениях списка', () => {
    vi.useFakeTimers()
    let addresses: NetworkAddress[] = []
    const changes: NetworkAddress[][] = []
    const monitor = new NetworkMonitor(
      () => addresses,
      (next) => changes.push(next),
      1000,
    )

    monitor.start()
    expect(changes).toHaveLength(1)

    addresses = [
      {
        interfaceName: 'wlan0',
        address: '192.168.1.5',
        apiUrl: 'http://192.168.1.5:8778',
        webUrl: 'http://192.168.1.5:8777',
      },
    ]
    vi.advanceTimersByTime(1000)
    expect(changes).toHaveLength(2)

    vi.advanceTimersByTime(1000)
    expect(changes).toHaveLength(2)
    monitor.stop()
  })
})
