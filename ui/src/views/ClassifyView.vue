<script setup lang="ts">
import { ref, onMounted } from "vue";
import { useStore, type Prediction } from "../stores/useStore";

const store = useStore();

const loading = ref(false);
const error = ref("");
const file = ref<File | null>(null);
const preview = ref<string | null>(null);
const prediction = ref<Prediction | null>(null);
const dragActive = ref(false);

const skystateColors: Record<string, string> = {
  clear: "#fbbf24",
  light_clouds: "#60a5fa",
  heavy_clouds: "#6b7280",
  precipitation: "#3b82f6",
  unknown: "#8b5cf6",
};

const skystateIcons: Record<string, string> = {
  clear: "mdi-weather-sunny",
  light_clouds: "mdi-weather-partly-cloudy",
  heavy_clouds: "mdi-weather-cloudy",
  precipitation: "mdi-weather-rainy",
  unknown: "mdi-help-circle",
};

function handleDrop(e: DragEvent) {
  dragActive.value = false;
  const droppedFile = e.dataTransfer?.files[0];
  if (droppedFile && droppedFile.type.startsWith("image/")) {
    selectFile(droppedFile);
  }
}

function handleFileInput(e: Event) {
  const input = e.target as HTMLInputElement;
  const selectedFile = input.files?.[0];
  if (selectedFile) {
    selectFile(selectedFile);
  }
}

function selectFile(f: File) {
  file.value = f;
  prediction.value = null;
  error.value = "";
  if (preview.value) {
    URL.revokeObjectURL(preview.value);
  }
  preview.value = URL.createObjectURL(f);
}

async function classify() {
  if (!file.value) return;

  loading.value = true;
  error.value = "";
  prediction.value = null;

  const result = await store.classifyImage(file.value);
  if (result) {
    prediction.value = result;
  } else {
    error.value = "Classification failed";
  }
  loading.value = false;
}

function clear() {
  file.value = null;
  prediction.value = null;
  error.value = "";
  if (preview.value) {
    URL.revokeObjectURL(preview.value);
    preview.value = null;
  }
}

onMounted(() => {
  store.fetchModelInfo();
});
</script>

<template>
  <div class="classify-view">
    <header class="header">
      <h1>Classify Image</h1>
      <p class="subtitle">
        Upload an image to classify using
        <strong>{{ store.modelInfo.value.active || "no model" }}</strong>
      </p>
    </header>

    <div class="content">
      <!-- Drop Zone -->
      <div
        v-if="!file"
        class="drop-zone"
        :class="{ active: dragActive }"
        @dragenter.prevent="dragActive = true"
        @dragleave.prevent="dragActive = false"
        @dragover.prevent
        @drop.prevent="handleDrop"
      >
        <span class="mdi mdi-cloud-upload"></span>
        <p>Drag & drop an image here</p>
        <span class="or">or</span>
        <label class="browse-btn">
          <input type="file" accept="image/*" @change="handleFileInput" />
          Browse Files
        </label>
      </div>

      <!-- Preview & Result -->
      <div v-else class="result-panel">
        <div class="image-preview">
          <img :src="preview!" alt="Preview" />
          <button class="clear-btn" @click="clear">
            <span class="mdi mdi-close"></span>
          </button>
        </div>

        <div class="actions">
          <button
            class="classify-btn"
            :disabled="loading || !store.modelInfo.value.active"
            @click="classify"
          >
            <span
              class="mdi"
              :class="loading ? 'mdi-loading mdi-spin' : 'mdi-brain'"
            ></span>
            {{ loading ? "Classifying..." : "Classify" }}
          </button>
        </div>

        <!-- Error -->
        <div v-if="error" class="error">
          <span class="mdi mdi-alert-circle"></span>
          {{ error }}
        </div>

        <!-- Prediction Result -->
        <div v-if="prediction" class="prediction">
          <div
            class="prediction-main"
            :style="{ '--sky-color': skystateColors[prediction.skystate] }"
          >
            <span :class="['mdi', skystateIcons[prediction.skystate]]"></span>
            <div>
              <span class="sky-label">{{
                prediction.skystate.replace("_", " ")
              }}</span>
              <span v-if="prediction.confidence" class="confidence">
                {{ (prediction.confidence * 100).toFixed(1) }}% confidence
              </span>
            </div>
          </div>

          <!-- Probability Bars -->
          <div v-if="prediction.probs" class="probs">
            <div
              v-for="(prob, key) in prediction.probs"
              :key="key"
              class="prob-row"
            >
              <span class="prob-label">{{
                String(key).replace("_", " ")
              }}</span>
              <div class="prob-bar">
                <div
                  class="prob-fill"
                  :style="{
                    width: `${prob * 100}%`,
                    background: skystateColors[String(key)] || '#6366f1',
                  }"
                ></div>
              </div>
              <span class="prob-value">{{ (prob * 100).toFixed(1) }}%</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.classify-view {
  max-width: 700px;
  margin: 0 auto;
}

.header {
  margin-bottom: 2rem;
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

.subtitle strong {
  color: #c7d2fe;
}

.drop-zone {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 350px;
  border: 2px dashed #27272a;
  border-radius: 20px;
  background: #0f0f14;
  transition: all 0.2s;
}

.drop-zone.active {
  border-color: #6366f1;
  background: rgba(99, 102, 241, 0.05);
}

.drop-zone .mdi {
  font-size: 4rem;
  color: #3f3f46;
  margin-bottom: 1rem;
}

.drop-zone p {
  color: #71717a;
  font-size: 1.1rem;
  margin: 0;
}

.or {
  color: #52525b;
  margin: 1rem 0;
  font-size: 0.85rem;
}

.browse-btn {
  padding: 0.75rem 1.5rem;
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 10px;
  color: #a1a1aa;
  font-size: 0.95rem;
  cursor: pointer;
  transition: all 0.2s;
}

.browse-btn:hover {
  background: #27272a;
  color: #fff;
}

.browse-btn input {
  display: none;
}

.result-panel {
  display: flex;
  flex-direction: column;
  gap: 1.5rem;
}

.image-preview {
  position: relative;
  border-radius: 16px;
  overflow: hidden;
  background: #0a0a0f;
}

.image-preview img {
  width: 100%;
  max-height: 400px;
  object-fit: contain;
  display: block;
}

.clear-btn {
  position: absolute;
  top: 1rem;
  right: 1rem;
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.6);
  border: none;
  border-radius: 50%;
  color: #fff;
  cursor: pointer;
  transition: all 0.2s;
}

.clear-btn:hover {
  background: rgba(220, 38, 38, 0.8);
}

.actions {
  display: flex;
  justify-content: center;
}

.classify-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 1rem 2rem;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  border: none;
  border-radius: 12px;
  color: #fff;
  font-size: 1.1rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}

.classify-btn:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 10px 25px rgba(99, 102, 241, 0.35);
}

.classify-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.error {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 1rem;
  background: rgba(220, 38, 38, 0.1);
  border: 1px solid rgba(220, 38, 38, 0.3);
  border-radius: 12px;
  color: #f87171;
}

.prediction {
  background: #0f0f14;
  border: 1px solid #1f1f28;
  border-radius: 16px;
  padding: 1.5rem;
}

.prediction-main {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding-bottom: 1.25rem;
  border-bottom: 1px solid #1f1f28;
  margin-bottom: 1.25rem;
}

.prediction-main .mdi {
  font-size: 3rem;
  color: var(--sky-color);
}

.sky-label {
  display: block;
  font-size: 1.5rem;
  font-weight: 700;
  color: #fafafa;
  text-transform: capitalize;
}

.confidence {
  display: block;
  font-size: 0.9rem;
  color: #71717a;
  margin-top: 0.25rem;
}

.probs {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.prob-row {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.prob-label {
  width: 100px;
  font-size: 0.85rem;
  color: #a1a1aa;
  text-transform: capitalize;
}

.prob-bar {
  flex: 1;
  height: 8px;
  background: #18181b;
  border-radius: 999px;
  overflow: hidden;
}

.prob-fill {
  height: 100%;
  border-radius: 999px;
  transition: width 0.3s ease;
}

.prob-value {
  width: 50px;
  text-align: right;
  font-size: 0.85rem;
  color: #71717a;
  font-variant-numeric: tabular-nums;
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
