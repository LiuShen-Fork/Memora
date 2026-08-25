<script setup lang="ts">
const { loggedIn, user, clear } = useUserSession()
const slotProps = computed(() => ({ loggedIn: loggedIn.value, user: user.value, clear }))

onMounted(() => {
  void useUserSession().fetch()
})
</script>

<template>
  <slot v-bind="slotProps" />
</template>
