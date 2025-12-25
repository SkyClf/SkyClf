<script setup lang="ts">
import type { ImageItem } from "../../stores/useStore";

defineProps<{
  image: ImageItem;
  loading?: boolean;
}>();

function formatTime(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}
</script>

<template>
  <div class="image-card" :class="{ loading }">
    <img :src="`/api/images/${image.id}`" :alt="image.id" />

    <div class="overlay">
      <div class="info">
        <span class="time">
          <span class="mdi mdi-clock-outline"></span>
          {{ formatTime(image.fetched_at) }}
        </span>
        <span v-if="image.skystate" class="label" :data-state="image.skystate">
          {{ image.skystate.replace("_", " ") }}
        </span>
      </div>
    </div>

    <div v-if="loading" class="loading-overlay">
      <span class="mdi mdi-loading mdi-spin"></span>
    </div>
  </div>
</template>

<style scoped>
.image-card {
  position: relative;
  border-radius: 16px;
  overflow: hidden;
  background: #0a0a0f;
  aspect-ratio: 16 / 9;
}

.image-card img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.overlay {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  padding: 1rem;
  background: linear-gradient(transparent, rgba(0, 0, 0, 0.8));
}

.info {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.time {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.9rem;
  color: rgba(255, 255, 255, 0.8);
}

.label {
  padding: 0.35rem 0.75rem;
  background: rgba(255, 255, 255, 0.15);
  border-radius: 999px;
  font-size: 0.8rem;
  font-weight: 500;
  color: #fff;
  text-transform: capitalize;
}

.label[data-state="clear"] {
  background: rgba(251, 191, 36, 0.2);
  color: #fbbf24;
}

.label[data-state="light_clouds"] {
  background: rgba(96, 165, 250, 0.2);
  color: #60a5fa;
}

.label[data-state="heavy_clouds"] {
  background: rgba(107, 114, 128, 0.3);
  color: #9ca3af;
}

.label[data-state="precipitation"] {
  background: rgba(59, 130, 246, 0.2);
  color: #3b82f6;
}

.label[data-state="unknown"] {
  background: rgba(139, 92, 246, 0.2);
  color: #8b5cf6;
}

.loading-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.5);
}

.loading-overlay .mdi {
  font-size: 2.5rem;
  color: #fff;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.mdi-spin {
  animation: spin 1s linear infinite;
}
</style>
