<template>
  <!--
    只有插槽的一层布局：页面自己负责整屏的外壳（照片墙是全屏容器，相册列表
    自己滚动），这里不再套任何 div。

    它的存在是为了让每个页面都处在一层布局里。`definePageMeta({ layout: false })`
    的页面在跳去有布局的页面时，旧页面会被拆两遍——第二遍再遍历已经从 DOM 上
    摘掉的 teleport 内容，就会抛出 `Cannot read properties of null (reading
    'nextSibling')`，新页面随之白屏。空布局让它走正常的布局切换。
  -->
  <slot />
</template>
