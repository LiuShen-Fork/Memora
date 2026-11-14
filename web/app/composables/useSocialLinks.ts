/**
 * 首页页脚那一排社交图标。
 *
 * 设置里存的是一段 JSON 数组文本（`app:socialLinks`），服务器已经校验过
 * 每一条都是 https 地址；这里只负责把它变成图标和平台名。认不出的地址仍
 * 然会显示，用一个通用的链接图标——自定义站点同样应该能挂上去。
 */
export interface SocialLink {
  href: string
  label: string
  icon: string
}

const platforms: { domains: string[]; label: string; icon: string }[] = [
  { domains: ['github.com'], label: 'GitHub', icon: 'tabler:brand-github' },
  { domains: ['x.com', 'twitter.com'], label: 'X', icon: 'tabler:brand-twitter' },
  { domains: ['instagram.com'], label: 'Instagram', icon: 'tabler:brand-instagram' },
  { domains: ['facebook.com', 'fb.com'], label: 'Facebook', icon: 'tabler:brand-facebook' },
  { domains: ['youtube.com', 'youtu.be'], label: 'YouTube', icon: 'tabler:brand-youtube' },
  { domains: ['bilibili.com', 'b23.tv'], label: 'Bilibili', icon: 'tabler:brand-bilibili' },
  { domains: ['weibo.com'], label: 'Weibo', icon: 'tabler:brand-weibo' },
  { domains: ['zhihu.com'], label: '知乎', icon: 'tabler:brand-zhihu' },
  { domains: ['t.me', 'telegram.org'], label: 'Telegram', icon: 'tabler:brand-telegram' },
  { domains: ['discord.com', 'discord.gg'], label: 'Discord', icon: 'tabler:brand-discord' },
  { domains: ['linkedin.com'], label: 'LinkedIn', icon: 'tabler:brand-linkedin' },
  { domains: ['tiktok.com'], label: 'TikTok', icon: 'tabler:brand-tiktok' },
  { domains: ['threads.net', 'threads.com'], label: 'Threads', icon: 'tabler:brand-threads' },
  { domains: ['bsky.app'], label: 'Bluesky', icon: 'tabler:brand-bluesky' },
  { domains: ['mastodon.social', 'mastodon.online'], label: 'Mastodon', icon: 'tabler:brand-mastodon' },
  { domains: ['pinterest.com'], label: 'Pinterest', icon: 'tabler:brand-pinterest' },
  { domains: ['reddit.com'], label: 'Reddit', icon: 'tabler:brand-reddit' },
  { domains: ['twitch.tv'], label: 'Twitch', icon: 'tabler:brand-twitch' },
  { domains: ['flickr.com'], label: 'Flickr', icon: 'tabler:brand-flickr' },
  { domains: ['vimeo.com'], label: 'Vimeo', icon: 'tabler:brand-vimeo' },
  { domains: ['behance.net'], label: 'Behance', icon: 'tabler:brand-behance' },
  { domains: ['dribbble.com'], label: 'Dribbble', icon: 'tabler:brand-dribbble' },
  { domains: ['tumblr.com'], label: 'Tumblr', icon: 'tabler:brand-tumblr' },
  { domains: ['snapchat.com'], label: 'Snapchat', icon: 'tabler:brand-snapchat' },
  { domains: ['medium.com'], label: 'Medium', icon: 'tabler:brand-medium' },
  { domains: ['qq.com'], label: 'QQ', icon: 'tabler:brand-qq' },
]

/**
 * 把设置里的 JSON 文本读成地址列表。
 *
 * `keepEmpty` 只给后台的编辑表单用：那里刚点「添加链接」的那一行还是空的，
 * 关掉它就会把这一行立刻过滤掉，按钮看起来像没反应。前台展示永远不要空行。
 */
export function parseSocialLinks(value: unknown, keepEmpty = false): string[] {
  let list: unknown = value
  if (typeof value === 'string') {
    if (!value.trim()) return []
    try {
      list = JSON.parse(value)
    } catch {
      return []
    }
  }
  if (!Array.isArray(list)) return []
  return list.filter(
    (link): link is string =>
      typeof link === 'string' && (keepEmpty || link.trim() !== ''),
  )
}

export function resolveSocialPlatform(href: string): { label: string; icon: string } {
  let host = ''
  try {
    host = new URL(href).hostname.toLowerCase().replace(/^www\./, '')
  } catch {
    return { label: href, icon: 'tabler:link' }
  }
  for (const platform of platforms) {
    if (platform.domains.some((domain) => host === domain || host.endsWith(`.${domain}`))) {
      return { label: platform.label, icon: platform.icon }
    }
  }
  return { label: host, icon: 'tabler:link' }
}

export function useSocialLinks() {
  const setting = useSettingRef('app:socialLinks')
  const links = computed<SocialLink[]>(() =>
    parseSocialLinks(setting.value).map((href) => ({
      href,
      ...resolveSocialPlatform(href),
    })),
  )
  return { links }
}
