<script lang="ts" setup>
/**
 * 照片墙页头里的那一簇按钮：主题、语言、排序、筛选、地图、相册，以及登录与后台。
 *
 * 紧凑顶栏和大页头用的是同一批按钮，区别只在摆在哪，所以按钮连同它们的样式
 * 一起放在这里 —— 两份页头共用一份，改一处两处一起变。
 *
 * `align="center"` 是给大页头用的：它把整簇按钮居中放进页头的竖排里；
 * 紧凑顶栏默认靠右，和左边的头像标题分列两端。
 */
withDefaults(
  defineProps<{
    align?: "end" | "center";
  }>(),
  { align: "end" },
);

const router = useRouter();
const { themeIcon, cycleTheme } = useThemeCycle();

const openLogin = () => router.push("/signin");
</script>

<template>
  <AuthState>
    <template #default="{ loggedIn, clear }">
      <div
        class="masonry-topbar__actions"
        :class="{ 'is-centered': align === 'center' }"
      >
        <div class="masonry-topbar__group masonry-topbar__functional-group">
          <UTooltip :text="$t('ui.action.theme.tooltip')" ignore-non-keyboard-focus>
            <UButton
              variant="ghost"
              color="neutral"
              class="masonry-topbar__button"
              :icon="themeIcon"
              size="sm"
              :aria-label="$t('ui.action.theme.tooltip')"
              @click="cycleTheme"
            />
          </UTooltip>
          <LanguageSwitcher compact button-class="masonry-topbar__button" />
          <MasonryItemTopbarSortMenu />
          <MasonryItemTopbarFilterMenu />
        </div>

        <div class="masonry-topbar__compact-functions masonry-topbar__group">
          <UPopover>
            <UButton
              variant="ghost"
              color="neutral"
              class="masonry-topbar__button"
              icon="tabler:dots-vertical"
              size="sm"
              aria-label="More actions"
            />
            <template #content>
              <UCard
                variant="glassmorphism"
                class="max-w-[calc(100vw-1rem)]"
                :ui="{ body: 'p-1 sm:p-1' }"
              >
                <div class="masonry-topbar__compact-function-row">
                  <UTooltip :text="$t('ui.action.theme.tooltip')" ignore-non-keyboard-focus>
                    <UButton
                      variant="ghost"
                      color="neutral"
                      class="masonry-topbar__button"
                      :icon="themeIcon"
                      size="sm"
                      :aria-label="$t('ui.action.theme.tooltip')"
                      @click="cycleTheme"
                    />
                  </UTooltip>
                  <LanguageSwitcher compact button-class="masonry-topbar__button" />
                  <MasonryItemTopbarSortMenu />
                  <MasonryItemTopbarFilterMenu />
                </div>
              </UCard>
            </template>
          </UPopover>
        </div>

        <div class="masonry-topbar__group masonry-topbar__types">
          <UTooltip :text="$t('ui.action.globe.tooltip')" ignore-non-keyboard-focus>
            <UButton
              variant="ghost"
              color="neutral"
              class="masonry-topbar__button"
              icon="tabler:map-pin-2"
              size="sm"
              :aria-label="$t('ui.action.globe.tooltip')"
              to="/globe"
            />
          </UTooltip>
          <UTooltip :text="$t('title.albums')" ignore-non-keyboard-focus>
            <UButton
              variant="ghost"
              color="neutral"
              class="masonry-topbar__button"
              icon="tabler:photo"
              size="sm"
              :aria-label="$t('title.albums')"
              to="/albums"
            />
          </UTooltip>
        </div>

        <div class="masonry-topbar__group">
          <template v-if="loggedIn">
            <UTooltip :text="$t('ui.action.dashboard.tooltip')" ignore-non-keyboard-focus>
              <UButton
                data-tour="home-dashboard"
                variant="ghost"
                color="neutral"
                class="masonry-topbar__button"
                icon="tabler:dashboard"
                size="sm"
                :aria-label="$t('ui.action.dashboard.tooltip')"
                to="/dashboard"
              />
            </UTooltip>
            <UTooltip :text="$t('ui.action.logout.tooltip')" ignore-non-keyboard-focus>
              <UButton
                variant="ghost"
                color="neutral"
                class="masonry-topbar__button"
                icon="tabler:logout"
                size="sm"
                :aria-label="$t('ui.action.logout.tooltip')"
                @click="clear"
              />
            </UTooltip>
          </template>
          <UTooltip v-else :text="$t('auth.form.signin.title')" ignore-non-keyboard-focus>
            <UButton
              variant="ghost"
              color="neutral"
              class="masonry-topbar__button"
              icon="tabler:login"
              size="sm"
              :aria-label="$t('auth.form.signin.title')"
              @click="openLogin"
            />
          </UTooltip>
        </div>
      </div>
    </template>
  </AuthState>
</template>

<style scoped>
.masonry-topbar__actions {
  display: flex;
  flex: 1 1 0%;
  justify-content: flex-end;
  gap: 0.35rem;
  min-width: 0;
  pointer-events: auto;
}

/* 大页头里按钮居中排在自己那一行，不再和身份信息分列两端。 */
.masonry-topbar__actions.is-centered {
  flex: 0 0 auto;
  justify-content: center;
  flex-wrap: wrap;
  min-width: auto;
}

.masonry-topbar__group {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 0.1rem;
  padding: 0.12rem;
  border: 1px solid rgb(255 255 255 / 35%);
  border-radius: 0.55rem;
  background: rgb(255 255 255 / 62%);
  box-shadow: 0 5px 18px rgb(0 0 0 / 5%);
  -webkit-backdrop-filter: blur(14px);
  backdrop-filter: blur(14px);
}

.dark .masonry-topbar__group {
  border-color: rgb(255 255 255 / 10%);
  background: rgb(24 24 27 / 55%);
  box-shadow: 0 5px 18px rgb(0 0 0 / 18%);
}

:deep(.masonry-topbar__button) {
  width: 1.8rem;
  height: 1.8rem;
  flex: 0 0 auto;
  cursor: pointer;
  justify-content: center;
  border-radius: 0.4rem;
  background: transparent !important;
  padding: 0 !important;
  transition:
    background-color 150ms ease,
    color 150ms ease;
}

:deep(.masonry-topbar__button:hover),
:deep(.masonry-topbar__button:focus-visible),
:deep(.masonry-topbar__button[data-state="open"]) {
  background: rgb(0 0 0 / 8%) !important;
}

.dark :deep(.masonry-topbar__button:hover),
.dark :deep(.masonry-topbar__button:focus-visible),
.dark :deep(.masonry-topbar__button[data-state="open"]) {
  background: rgb(255 255 255 / 12%) !important;
}

.masonry-topbar__compact-functions {
  display: none;
}

.masonry-topbar__compact-function-row {
  display: flex;
  align-items: center;
  gap: 0.15rem;
  white-space: nowrap;
}

@media (max-width: 768px) {
  .masonry-topbar__actions {
    flex: 0 1 auto;
    max-width: none;
    gap: 0.2rem;
    overflow-x: auto;
    padding-bottom: 1px;
    scrollbar-width: none;
  }

  .masonry-topbar__actions::-webkit-scrollbar {
    display: none;
  }

  .masonry-topbar__group {
    padding: 0.12rem;
    border-radius: 0.5rem;
  }

  :deep(.masonry-topbar__button) {
    width: 1.65rem;
    height: 1.65rem;
  }
}

@media (max-width: 640px) {
  .masonry-topbar__functional-group {
    display: none;
  }

  .masonry-topbar__compact-functions {
    display: inline-flex;
  }
}
</style>
