import type { ProxyBrowsersResponse } from '@/types'
import type { systemKeys, useProxyBrowsers } from './use-system'

type Assert<T extends true> = T
type IsEqual<A, B> = (<T>() => T extends A ? 1 : 2) extends <T>() => T extends B ? 1 : 2
  ? true
  : false

export type ProxyBrowsersHookDataContract = Assert<
  NonNullable<ReturnType<typeof useProxyBrowsers>['data']> extends ProxyBrowsersResponse ? true : false
>

export type ProxyBrowsersQueryKeyContract = Assert<
  IsEqual<typeof systemKeys.proxyBrowsers, readonly ['proxy', 'browsers']>
>

export type ProxyBrowsersResponseContract = Assert<
  ProxyBrowsersResponse extends {
    default_browser: string
    default_user_agent: string
    browsers: Array<{
      browser: string
      label: string
      user_agent: string
    }>
  }
    ? true
    : false
>
