<script setup lang="ts">
import { ref, watch, nextTick } from "vue";
import type { ImageItem } from "../../stores/useStore";

const props = defineProps<{
  images: ImageItem[];
  currentIndex: number;
}>();

const emit = defineEmits<{
  select: [index: number];
}>();

const container = ref<HTMLElement | null>(null);

// Auto-scroll to current image
watch(
  () => props.currentIndex,
  async () => {
    await nextTick();
    const el = container.value?.querySelector(".thumb.active") as HTMLElement;
    if (el) {
      el.scrollIntoView({
        behavior: "smooth",
        block: "nearest",
        inline: "center",
      });
    }
  }
);
</script>

<template>
  <div class="timeline" ref="container">
    <button
      v-for="(img, idx) in images"
      :key="img.id"
      :class="[
        'thumb',
        { active: idx === currentIndex, labeled: !!img.skystate },
      ]"
      @click="emit('select', idx)"
    >
      <img :src="`/api/images/${img.id}`" :alt="img.id" loading="lazy" />
      <span v-if="img.skystate" class="dot" :data-state="img.skystate"></span>
    </button>

    <div v-if="images.length === 0" class="empty">No images</div>
  </div>
</template>

<style scoped>
.timeline {
  display: flex;
  gap: 0.5rem;
  overflow-x: auto;
  padding-bottom: 0.5rem;
  max-height: 400px;
  flex-wrap: wrap;
  align-content: flex-start;
}

.thumb {
  position: relative;
  width: 64px;
  height: 48px;
  border: 2px solid transparent;
  border-radius: 8px;
  overflow: hidden;
  cursor: pointer;
  flex-shrink: 0;
  background: #18181b;
  padding: 0;
  transition: all 0.15s;
}

.thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  opacity: 0.7;
  transition: opacity 0.15s;
}

.thumb:hover img {
  opacity: 1;
}

.thumb.active {
  border-color: #6366f1;
  box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.3);
}

.thumb.active img {
  opacity: 1;
}

.dot {
  position: absolute;
  bottom: 3px;
  right: 3px;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #22c55e;
}

.dot[data-state="clear"] {
  background: #fbbf24;
}

.dot[data-state="light_clouds"] {
  background: #60a5fa;
}

.dot[data-state="heavy_clouds"] {
  background: #6b7280;
}

.dot[data-state="precipitation"] {
  background: #3b82f6;
}

.dot[data-state="unknown"] {
  background: #8b5cf6;
}

.empty {
  color: #52525b;
  font-size: 0.85rem;
  padding: 1rem;
  text-align: center;
  width: 100%;
}

/* Scrollbar */
.timeline::-webkit-scrollbar {
  height: 6px;
}

.timeline::-webkit-scrollbar-track {
  background: transparent;
}

.timeline::-webkit-scrollbar-thumb {
  background: #27272a;
  border-radius: 3px;
}
</style>
