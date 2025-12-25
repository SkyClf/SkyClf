<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch, computed } from "vue";
import { useStore } from "../stores/useStore";
import ImageCard from "../components/v2/ImageCard.vue";
import Timeline from "../components/v2/Timeline.vue";
import StatsPanel from "../components/v2/StatsPanel.vue";

const store = useStore();
const showUnlabeledOnly = ref(true);
const labeling = ref(false);

const skystates = [
  {
    value: "clear",
    label: "Clear",
    icon: "mdi-weather-sunny",
    color: "#fbbf24",
    key: "1",
  },
  {
    value: "light_clouds",
    label: "Light",
    icon: "mdi-weather-partly-cloudy",
    color: "#60a5fa",
    key: "2",
  },
  {
    value: "heavy_clouds",
    label: "Heavy",
    icon: "mdi-weather-cloudy",
    color: "#6b7280",
    key: "3",
  },
  {
    value: "precipitation",
    label: "Rain",
    icon: "mdi-weather-rainy",
    color: "#3b82f6",
    key: "4",
  },
  {
    value: "unknown",
    label: "Unknown",
    icon: "mdi-help-circle",
    color: "#8b5cf6",
    key: "5",
  },
];

const currentDaySummary = computed(() => {
  return store.days.value.find((d) => d.date === store.selectedDay.value);
});

async function loadData() {
  await store.fetchDays();
  await store.fetchStats();
  await loadImages();
}

async function loadImages() {
  await store.fetchImages({
    unlabeled: showUnlabeledOnly.value,
    day: store.selectedDay.value,
  });
}

async function handleLabel(skystate: string, meteor = false) {
  const img = store.currentImage.value;
  if (!img || labeling.value) return;

  labeling.value = true;
  const success = await store.setLabel(img.id, skystate, meteor);

  if (success) {
    if (showUnlabeledOnly.value) {
      store.images.value.splice(store.currentIndex.value, 1);
      if (store.currentIndex.value >= store.images.value.length) {
        store.currentIndex.value = Math.max(0, store.images.value.length - 1);
      }
    } else {
      store.nextImage();
    }
    store.stats.value.labeled++;
    store.stats.value.unlabeled = Math.max(0, store.stats.value.unlabeled - 1);
  }
  labeling.value = false;
}

function handleKeydown(e: KeyboardEvent) {
  if ((e.target as HTMLElement).tagName === "INPUT") return;

  const keyIndex = parseInt(e.key) - 1;
  if (keyIndex >= 0 && keyIndex < skystates.length) {
    handleLabel(skystates[keyIndex]!.value);
    return;
  }

  if (e.key === "ArrowRight" || e.key === "d" || e.key === " ") {
    e.preventDefault();
    store.nextImage();
  }
  if (e.key === "ArrowLeft" || e.key === "a") {
    e.preventDefault();
    store.prevImage();
  }
  if (e.key === "m" || e.key === "M") {
    handleLabel(store.currentImage.value?.skystate || "unknown", true);
  }
}

watch([() => store.selectedDay.value, showUnlabeledOnly], () => {
  store.currentIndex.value = 0;
  loadImages();
});

let pollInterval: number | null = null;

onMounted(() => {
  loadData();
  window.addEventListener("keydown", handleKeydown);
  pollInterval = window.setInterval(() => store.fetchStats(), 5000);
});

onUnmounted(() => {
  window.removeEventListener("keydown", handleKeydown);
  if (pollInterval) clearInterval(pollInterval);
});
</script>

<template>
  <div class="label-view">
    <!-- Header -->
    <header class="header">
      <div class="header-left">
        <h1>Label Images</h1>
        <p class="subtitle">
          {{ store.images.value.length }} images
          <span v-if="currentDaySummary">
            · {{ store.formatBytes(currentDaySummary.size_bytes) }}
          </span>
        </p>
      </div>

      <div class="header-controls">
        <select v-model="store.selectedDay.value" class="day-select">
          <option :value="null">All days</option>
          <option
            v-for="day in store.days.value"
            :key="day.date"
            :value="day.date"
          >
            {{ day.date }} ({{ day.count }})
          </option>
        </select>

        <label class="toggle">
          <input type="checkbox" v-model="showUnlabeledOnly" />
          <span class="toggle-track"></span>
          <span class="toggle-label">Unlabeled only</span>
        </label>
      </div>
    </header>

    <!-- Main Grid -->
    <div class="content-grid">
      <!-- Left: Image -->
      <div class="image-section">
        <ImageCard
          v-if="store.currentImage.value"
          :image="store.currentImage.value"
          :loading="labeling"
        />
        <div v-else class="empty-state">
          <span class="mdi mdi-image-off"></span>
          <p>No images to label</p>
        </div>

        <!-- Label Buttons -->
        <div class="label-buttons">
          <button
            v-for="s in skystates"
            :key="s.value"
            class="label-btn"
            :style="{ '--btn-color': s.color }"
            :disabled="!store.currentImage.value || labeling"
            @click="handleLabel(s.value)"
          >
            <span :class="['mdi', s.icon]"></span>
            <span class="label-text">{{ s.label }}</span>
            <kbd>{{ s.key }}</kbd>
          </button>
        </div>

        <!-- Navigation -->
        <div class="nav-row">
          <button
            class="nav-btn"
            :disabled="!store.hasPrev.value"
            @click="store.prevImage()"
          >
            <span class="mdi mdi-chevron-left"></span>
            Previous
          </button>
          <span class="nav-counter">
            {{ store.currentIndex.value + 1 }} / {{ store.images.value.length }}
          </span>
          <button
            class="nav-btn"
            :disabled="!store.hasNext.value"
            @click="store.nextImage()"
          >
            Next
            <span class="mdi mdi-chevron-right"></span>
          </button>
        </div>
      </div>

      <!-- Right: Stats & Timeline -->
      <div class="side-section">
        <StatsPanel :stats="store.stats.value" />

        <div class="timeline-container">
          <h3>Timeline</h3>
          <Timeline
            :images="store.images.value"
            :current-index="store.currentIndex.value"
            @select="store.setIndex"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.label-view {
  max-width: 1400px;
  margin: 0 auto;
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 2rem;
  flex-wrap: wrap;
  gap: 1rem;
}

.header h1 {
  font-size: 1.75rem;
  font-weight: 700;
  color: #fafafa;
  margin: 0;
}

.subtitle {
  color: #71717a;
  font-size: 0.9rem;
  margin-top: 0.25rem;
}

.header-controls {
  display: flex;
  align-items: center;
  gap: 1.5rem;
}

.day-select {
  background: #18181b;
  border: 1px solid #27272a;
  color: #fff;
  padding: 0.625rem 1rem;
  border-radius: 10px;
  font-size: 0.9rem;
  min-width: 180px;
}

.toggle {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  cursor: pointer;
}

.toggle input {
  display: none;
}

.toggle-track {
  width: 44px;
  height: 24px;
  background: #27272a;
  border-radius: 999px;
  position: relative;
  transition: background 0.2s;
}

.toggle-track::after {
  content: "";
  position: absolute;
  width: 18px;
  height: 18px;
  background: #52525b;
  border-radius: 50%;
  top: 3px;
  left: 3px;
  transition: all 0.2s;
}

.toggle input:checked + .toggle-track {
  background: #6366f1;
}

.toggle input:checked + .toggle-track::after {
  left: 23px;
  background: #fff;
}

.toggle-label {
  color: #a1a1aa;
  font-size: 0.9rem;
}

.content-grid {
  display: grid;
  grid-template-columns: 1fr 340px;
  gap: 2rem;
}

@media (max-width: 1100px) {
  .content-grid {
    grid-template-columns: 1fr;
  }
}

.image-section {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 400px;
  background: #0f0f14;
  border: 1px solid #1f1f28;
  border-radius: 16px;
  color: #52525b;
}

.empty-state .mdi {
  font-size: 4rem;
  margin-bottom: 1rem;
}

.label-buttons {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.label-btn {
  flex: 1;
  min-width: 100px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.5rem;
  padding: 1rem;
  background: #18181b;
  border: 2px solid transparent;
  border-radius: 14px;
  color: #a1a1aa;
  cursor: pointer;
  transition: all 0.2s;
}

.label-btn:hover:not(:disabled) {
  border-color: var(--btn-color);
  background: color-mix(in srgb, var(--btn-color) 10%, #18181b);
  color: #fff;
}

.label-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.label-btn .mdi {
  font-size: 1.75rem;
  color: var(--btn-color);
}

.label-text {
  font-weight: 600;
  font-size: 0.9rem;
}

.label-btn kbd {
  font-size: 0.7rem;
  background: #27272a;
  padding: 0.2rem 0.5rem;
  border-radius: 6px;
  color: #71717a;
}

.nav-row {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1.5rem;
}

.nav-btn {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem 1.25rem;
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 10px;
  color: #a1a1aa;
  font-size: 0.9rem;
  cursor: pointer;
  transition: all 0.2s;
}

.nav-btn:hover:not(:disabled) {
  background: #1f1f28;
  color: #fff;
}

.nav-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.nav-counter {
  color: #71717a;
  font-size: 0.9rem;
  font-variant-numeric: tabular-nums;
}

.side-section {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.timeline-container {
  background: #0f0f14;
  border: 1px solid #1f1f28;
  border-radius: 16px;
  padding: 1.25rem;
}

.timeline-container h3 {
  font-size: 0.9rem;
  font-weight: 600;
  color: #a1a1aa;
  margin: 0 0 1rem 0;
}
</style>
