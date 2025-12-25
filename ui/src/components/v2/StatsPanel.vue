<script setup lang="ts">
import type { DatasetStats } from "../../stores/useStore";
import { useStore } from "../../stores/useStore";

defineProps<{
  stats: DatasetStats;
}>();

const { formatBytes } = useStore();

const skystates = [
  { key: "clear", label: "Clear", color: "#fbbf24" },
  { key: "light_clouds", label: "Light Clouds", color: "#60a5fa" },
  { key: "heavy_clouds", label: "Heavy Clouds", color: "#6b7280" },
  { key: "precipitation", label: "Precipitation", color: "#3b82f6" },
  { key: "unknown", label: "Unknown", color: "#8b5cf6" },
];
</script>

<template>
  <div class="stats-panel">
    <div class="stat-row main">
      <div class="stat">
        <span class="stat-value">{{ stats.labeled }}</span>
        <span class="stat-label">Labeled</span>
      </div>
      <div class="stat">
        <span class="stat-value">{{ stats.unlabeled }}</span>
        <span class="stat-label">Unlabeled</span>
      </div>
      <div class="stat">
        <span class="stat-value">{{
          formatBytes(stats.total_size_bytes)
        }}</span>
        <span class="stat-label">Storage</span>
      </div>
    </div>

    <!-- Progress bar -->
    <div class="progress-section">
      <div class="progress-header">
        <span>Progress</span>
        <span
          >{{
            stats.total > 0
              ? Math.round((stats.labeled / stats.total) * 100)
              : 0
          }}%</span
        >
      </div>
      <div class="progress-bar">
        <div
          class="progress-fill"
          :style="{
            width:
              stats.total > 0
                ? `${(stats.labeled / stats.total) * 100}%`
                : '0%',
          }"
        ></div>
      </div>
    </div>

    <!-- Class breakdown -->
    <div class="class-breakdown">
      <h4>By Class</h4>
      <div class="class-list">
        <div v-for="s in skystates" :key="s.key" class="class-item">
          <span class="class-dot" :style="{ background: s.color }"></span>
          <span class="class-label">{{ s.label }}</span>
          <span class="class-count">{{ stats.by_class[s.key] || 0 }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.stats-panel {
  background: #0f0f14;
  border: 1px solid #1f1f28;
  border-radius: 16px;
  padding: 1.25rem;
}

.stat-row {
  display: flex;
  gap: 1rem;
  margin-bottom: 1.25rem;
}

.stat {
  flex: 1;
  text-align: center;
  padding: 0.75rem;
  background: #18181b;
  border-radius: 10px;
}

.stat-value {
  display: block;
  font-size: 1.25rem;
  font-weight: 700;
  color: #fafafa;
}

.stat-label {
  font-size: 0.75rem;
  color: #71717a;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.progress-section {
  margin-bottom: 1.25rem;
}

.progress-header {
  display: flex;
  justify-content: space-between;
  font-size: 0.8rem;
  color: #71717a;
  margin-bottom: 0.5rem;
}

.progress-bar {
  height: 6px;
  background: #27272a;
  border-radius: 999px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #6366f1, #8b5cf6);
  border-radius: 999px;
  transition: width 0.3s ease;
}

.class-breakdown h4 {
  font-size: 0.8rem;
  font-weight: 600;
  color: #71717a;
  margin: 0 0 0.75rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.class-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.class-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.85rem;
}

.class-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
}

.class-label {
  flex: 1;
  color: #a1a1aa;
}

.class-count {
  font-weight: 600;
  color: #fafafa;
  font-variant-numeric: tabular-nums;
}
</style>
