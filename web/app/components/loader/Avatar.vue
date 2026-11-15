<script lang="ts" setup>
/**
 * The loading mark: the site's avatar with a themed arc sweeping around it.
 *
 * The brand mark has transparent corners, so nothing clips it and nothing sits
 * behind it — a ring or a backplate around a half-transparent logo reads as a
 * hard border it does not have. The arc is drawn as an SVG dash instead of a
 * bordered box, which keeps the motion soft and lets the gradient travel along
 * the stroke.
 *
 * Colours come from `--ui-primary` / `--ui-secondary`, which Nuxt UI only
 * injects at runtime, so every use carries a static Tailwind palette fallback:
 * before that style lands the variables are undefined and an unresolvable
 * border colour falls back to `currentColor`, which is white in dark mode.
 */
const gradientId = useId()
</script>

<template>
  <div
    class="memora-loader relative size-24"
    role="status"
    :aria-label="$t('ui.loading')"
  >
    <span
      class="memora-loader__glow absolute inset-0 rounded-full"
      aria-hidden="true"
    />

    <svg
      class="memora-loader__arc absolute inset-0 size-full"
      viewBox="0 0 96 96"
      fill="none"
      aria-hidden="true"
    >
      <defs>
        <linearGradient
          :id="gradientId"
          x1="8"
          y1="8"
          x2="88"
          y2="88"
          gradientUnits="userSpaceOnUse"
        >
          <stop
            offset="0%"
            stop-color="var(--memora-loader-from)"
          />
          <stop
            offset="100%"
            stop-color="var(--memora-loader-to)"
          />
        </linearGradient>
      </defs>
      <circle
        cx="48"
        cy="48"
        r="45"
        :stroke="`url(#${gradientId})`"
        stroke-width="3"
        stroke-linecap="round"
        stroke-dasharray="66 217"
      />
    </svg>

    <img
      v-if="getSetting('app:avatarUrl')"
      class="memora-loader__mark absolute inset-0 m-auto size-[58%] object-contain"
      :src="getSetting('app:avatarUrl')?.toString()"
      alt=""
    />
    <img
      v-else
      class="memora-loader__mark absolute inset-0 m-auto size-[58%] object-contain"
      src="/web-app-manifest-192x192.png"
      alt=""
    />
  </div>
</template>

<style scoped>
.memora-loader {
  /* Runtime values first, static palette as the first-paint fallback. */
  --memora-loader-from: var(--ui-primary, var(--color-pink-500));
  --memora-loader-to: var(--ui-secondary, var(--color-purple-400));
}

:global(.dark) .memora-loader {
  --memora-loader-from: var(--ui-primary, var(--color-pink-400));
  --memora-loader-to: var(--ui-secondary, var(--color-purple-400));
}

.memora-loader__glow {
  background: radial-gradient(
    circle at center,
    var(--memora-loader-from),
    transparent 68%
  );
  opacity: 0.16;
  filter: blur(6px);
  animation: memora-loader-breathe 2.8s ease-in-out infinite;
}

.memora-loader__arc {
  /* Start the arc at twelve o'clock. */
  transform: rotate(-90deg);
  animation: memora-loader-orbit 1.3s cubic-bezier(0.6, 0.15, 0.35, 0.85)
    infinite;
  filter: drop-shadow(0 1px 6px rgb(0 0 0 / 0.08));
}

.memora-loader__mark {
  animation: memora-loader-breathe 2.8s ease-in-out infinite;
  filter: drop-shadow(0 2px 8px rgb(0 0 0 / 0.12));
}

@keyframes memora-loader-orbit {
  from {
    transform: rotate(-90deg);
  }

  to {
    transform: rotate(270deg);
  }
}

@keyframes memora-loader-breathe {
  0%,
  100% {
    transform: scale(0.97);
  }

  50% {
    transform: scale(1.03);
  }
}

@media (prefers-reduced-motion: reduce) {
  .memora-loader__glow,
  .memora-loader__arc,
  .memora-loader__mark {
    animation: none;
  }
}
</style>
