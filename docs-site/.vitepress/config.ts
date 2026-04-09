import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'MagiC',
  description: 'Kubernetes for AI agents — manage fleets of AI workers through an open protocol',
  base: '/docs/',
  outDir: '../site/docs',
  cleanUrls: true,

  head: [
    ['link', { rel: 'icon', href: '/docs/favicon.ico' }],
  ],

  themeConfig: {
    logo: '🪄',
    siteTitle: 'MagiC',

    nav: [
      { text: 'Guide', link: '/guide/getting-started' },
      { text: 'API Reference', link: '/api/reference' },
      { text: 'GitHub', link: 'https://github.com/kienbui1995/magic' },
    ],

    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'What is MagiC?', link: '/guide/what-is-magic' },
          { text: 'Quick Start', link: '/guide/getting-started' },
          { text: 'Core Concepts', link: '/guide/concepts' },
        ],
      },
      {
        text: 'Building Workers',
        items: [
          { text: 'Python SDK', link: '/guide/python-sdk' },
          { text: 'Go SDK', link: '/guide/go-sdk' },
          { text: 'HTTP Protocol', link: '/guide/protocol' },
        ],
      },
      {
        text: 'Production',
        items: [
          { text: 'Deployment', link: '/guide/deployment' },
          { text: 'Storage Backends', link: '/guide/storage' },
          { text: 'Webhooks', link: '/guide/webhooks' },
          { text: 'Streaming (SSE)', link: '/guide/streaming' },
          { text: 'Observability', link: '/guide/observability' },
        ],
      },
      {
        text: 'Reference',
        items: [
          { text: 'API Reference', link: '/api/reference' },
          { text: 'Configuration', link: '/api/configuration' },
        ],
      },
    ],

    socialLinks: [
      { icon: 'github', link: 'https://github.com/kienbui1995/magic' },
    ],

    footer: {
      message: 'Released under the Apache 2.0 License.',
      copyright: 'Copyright © 2026 MagiC',
    },

    search: {
      provider: 'local',
    },
  },
})
