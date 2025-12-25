<script setup lang="ts">
import { RouterView, useRoute, useRouter } from "vue-router";
import { computed } from "vue";
import logoSvg from "../assets/SkyClfLogo.svg";

const route = useRoute();
const router = useRouter();

const tabs = [
  { name: "label", label: "Label", icon: "mdi-tag-multiple" },
  { name: "train", label: "Train", icon: "mdi-brain" },
  { name: "classify", label: "Classify", icon: "mdi-image-search" },
];

const currentTab = computed(() => route.name as string);

function navigate(name: string) {
  router.push({ name });
}
</script>

<template>
  <div class="layout">
    <!-- Sidebar -->
    <aside class="sidebar">
      <div class="logo">
        <img :src="logoSvg" alt="SkyClf" />
        <span>SkyClf</span>
        <span class="version">v2</span>
      </div>

      <nav class="nav">
        <button
          v-for="tab in tabs"
          :key="tab.name"
          :class="['nav-btn', { active: currentTab === tab.name }]"
          @click="navigate(tab.name)"
        >
          <span :class="['mdi', tab.icon]"></span>
          <span>{{ tab.label }}</span>
        </button>
      </nav>

      <div class="sidebar-footer">
        <a href="/" class="legacy-link">
          <span class="mdi mdi-arrow-left"></span>
          Legacy UI
        </a>
      </div>
    </aside>

    <!-- Main Content -->
    <main class="main">
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  min-height: 100vh;
  background: #09090b;
}

.sidebar {
  width: 240px;
  background: #0f0f14;
  border-right: 1px solid #1f1f28;
  display: flex;
  flex-direction: column;
  padding: 1.5rem 1rem;
}

.logo {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0 0.5rem;
  margin-bottom: 2rem;
}

.logo img {
  width: 36px;
  height: 36px;
}

.logo span {
  font-size: 1.25rem;
  font-weight: 700;
  color: #fff;
}

.logo .version {
  font-size: 0.7rem;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  padding: 0.15rem 0.5rem;
  border-radius: 999px;
  font-weight: 600;
}

.nav {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.nav-btn {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.875rem 1rem;
  border: none;
  background: transparent;
  color: #71717a;
  font-size: 0.95rem;
  font-weight: 500;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s;
}

.nav-btn:hover {
  background: #18181b;
  color: #a1a1aa;
}

.nav-btn.active {
  background: linear-gradient(
    135deg,
    rgba(99, 102, 241, 0.15),
    rgba(139, 92, 246, 0.1)
  );
  color: #c7d2fe;
  border: 1px solid rgba(99, 102, 241, 0.3);
}

.nav-btn .mdi {
  font-size: 1.25rem;
}

.sidebar-footer {
  margin-top: auto;
  padding-top: 1rem;
  border-top: 1px solid #1f1f28;
}

.legacy-link {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: #52525b;
  font-size: 0.85rem;
  text-decoration: none;
  padding: 0.5rem;
}

.legacy-link:hover {
  color: #71717a;
}

.main {
  flex: 1;
  overflow-y: auto;
  padding: 2rem;
}
</style>
