<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Activity, ClipboardCheck, LandPlot, LogOut, Menu, Radar, ScrollText, X } from 'lucide-vue-next'
import { useAuth } from '@/hooks/useAuth'

const route = useRoute(); const router = useRouter(); const auth = useAuth(); const navOpen = ref(false)
const isLogin = computed(() => route.name === 'login')
const nav = computed(() => [
  { to: '/parcels', label: '地块管理', icon: LandPlot, show: true },
  { to: '/observations', label: '测量观测', icon: Activity, show: true },
  { to: '/proposals', label: '边界提案', icon: ClipboardCheck, show: true },
  { to: '/conflicts', label: '冲突消解', icon: Radar, show: true },
  { to: '/audit', label: '审计中心', icon: ScrollText, show: auth.canReview() || auth.hasRole('auditor') },
].filter((item) => item.show))
function leave() { auth.logout(); router.push('/login') }
</script>

<template>
  <router-view v-if="isLogin" />
  <div v-else class="app-frame">
    <header class="mobile-bar"><button class="plain-icon" :aria-label="navOpen ? '关闭导航' : '打开导航'" @click="navOpen = !navOpen"><X v-if="navOpen" /><Menu v-else /></button><strong>ParcelGraph</strong><span class="live-dot">离线分析</span></header>
    <aside class="sidebar" :class="{ open: navOpen }">
      <div class="brand"><div class="brand-mark"><Activity :size="22" /></div><div><strong>ParcelGraph</strong><span>CADASTRAL REVIEW DESK</span></div></div>
      <nav aria-label="主导航"><router-link v-for="item in nav" :key="item.to" :to="item.to" @click="navOpen = false"><component :is="item.icon" :size="18" /><span>{{ item.label }}</span></router-link></nav>
      <div class="system-boundary"><span class="boundary-light" />离线分析模式</div>
      <div class="account"><div class="avatar">{{ auth.user.value?.display_name.slice(0, 1) }}</div><div><strong>{{ auth.user.value?.display_name }}</strong><span>{{ auth.user.value?.role }}</span></div><el-tooltip content="退出登录"><button class="plain-icon" aria-label="退出登录" @click="leave"><LogOut :size="17" /></button></el-tooltip></div>
    </aside>
    <main class="main-content"><router-view /></main>
  </div>
</template>
