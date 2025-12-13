<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";

// ============ Types ============
interface TrainConfig {
  epochs: number;
  batch_size: number;
  lr: string;
  img_size: number;
}

interface TrainStatus {
  running: boolean;
  container_id?: string;
  started_at?: string;
  exit_code?: number;
  error?: string;
  logs?: string;
  last_config?: TrainConfig;
}

interface ImageItem {
  id: string;
  path: string;
  sha256: string;
  fetched_at: string;
  skystate?: string;
  meteor?: boolean;
  labeled_at?: string;
}

// ============ State ============
const activeTab = ref<"label" | "train">("label");

// Labeling state
const images = ref<ImageItem[]>([]);
const currentIndex = ref(0);
const showUnlabeledOnly = ref(true);
const labeledCount = ref(0);
const totalCount = ref(0);
const labeling = ref(false);

// Training state
const status = ref<TrainStatus>({ running: false });
const loading = ref(false);
const error = ref("");
const epochs = ref(10);
const batchSize = ref(16);
const learningRate = ref("0.001");

let pollInterval: number | null = null;

// ============ Computed ============
const currentImage = computed(() => images.value[currentIndex.value] || null);
const hasNext = computed(() => currentIndex.value < images.value.length - 1);
const hasPrev = computed(() => currentIndex.value > 0);

const skystateOptions = [
  { value: "clear", label: "☀️ Clear", key: "1" },
  { value: "light_clouds", label: "🌤️ Light Clouds", key: "2" },
  { value: "heavy_clouds", label: "☁️ Heavy Clouds", key: "3" },
  { value: "precipitation", label: "🌧️ Precipitation", key: "4" },
  { value: "unknown", label: "❓ Unknown", key: "5" },
];

// ============ Labeling Functions ============
async function fetchImages() {
  try {
    const params = new URLSearchParams({ limit: "500" });
    if (showUnlabeledOnly.value) params.set("unlabeled", "1");

    const res = await fetch(`/api/dataset/images?${params}`);
    if (res.ok) {
      const data = await res.json();
      images.value = data.items || [];
      totalCount.value = data.count || 0;
    }
  } catch (e) {
    console.error("Failed to fetch images:", e);
  }
}

async function fetchStats() {
  try {
    // Get total labeled count
    const res = await fetch("/api/dataset/images?limit=1000");
    if (res.ok) {
      const data = await res.json();
      const items = data.items || [];
      labeledCount.value = items.filter((i: ImageItem) => i.skystate).length;
    }
  } catch (e) {
    console.error("Failed to fetch stats:", e);
  }
}

async function setLabel(skystate: string, meteor: boolean = false) {
  if (!currentImage.value || labeling.value) return;

  labeling.value = true;
  try {
    const res = await fetch("/api/labels", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        image_id: currentImage.value.id,
        skystate,
        meteor,
      }),
    });

    if (res.ok) {
      // Update local state
      if (showUnlabeledOnly.value) {
        // Remove from list and stay at same index (show next image)
        images.value.splice(currentIndex.value, 1);
        if (
          currentIndex.value >= images.value.length &&
          images.value.length > 0
        ) {
          currentIndex.value = images.value.length - 1;
        }
      } else {
        // Update the label in place
        const img = images.value[currentIndex.value];
        if (img) {
          img.skystate = skystate;
          img.meteor = meteor;
        }
        nextImage();
      }
      labeledCount.value++;
    }
  } catch (e) {
    console.error("Failed to set label:", e);
  } finally {
    labeling.value = false;
  }
}

function nextImage() {
  if (hasNext.value) currentIndex.value++;
}

function prevImage() {
  if (hasPrev.value) currentIndex.value--;
}

function handleKeydown(e: KeyboardEvent) {
  if (activeTab.value !== "label" || !currentImage.value) return;

  // Number keys 1-5 for skystate
  const keyIndex = parseInt(e.key) - 1;
  if (keyIndex >= 0 && keyIndex < skystateOptions.length) {
    const opt = skystateOptions[keyIndex];
    if (opt) setLabel(opt.value);
    return;
  }

  // Arrow keys for navigation
  if (e.key === "ArrowRight" || e.key === "d") nextImage();
  if (e.key === "ArrowLeft" || e.key === "a") prevImage();

  // M for meteor toggle (with current or default skystate)
  if (e.key === "m" || e.key === "M") {
    const currentSkystate = currentImage.value.skystate || "unknown";
    setLabel(currentSkystate, true);
  }
}

// ============ Training Functions ============
async function fetchStatus() {
  try {
    const res = await fetch("/api/train/status");
    if (res.ok) {
      status.value = await res.json();
    }
  } catch (e) {
    console.error("Failed to fetch status:", e);
  }
}

async function startTraining() {
  loading.value = true;
  error.value = "";
  try {
    const res = await fetch("/api/train/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        epochs: epochs.value,
        batch_size: batchSize.value,
        lr: learningRate.value,
        img_size: 224,
        seed: 42,
        val_split: "0.2",
      }),
    });
    const data = await res.json();
    if (!res.ok) {
      error.value = data.error || "Failed to start training";
    }
    await fetchStatus();
  } catch (e) {
    error.value = "Network error";
  } finally {
    loading.value = false;
  }
}

async function stopTraining() {
  loading.value = true;
  error.value = "";
  try {
    const res = await fetch("/api/train/stop", { method: "POST" });
    const data = await res.json();
    if (!res.ok) {
      error.value = data.error || "Failed to stop training";
    }
    await fetchStatus();
  } catch (e) {
    error.value = "Network error";
  } finally {
    loading.value = false;
  }
}

// ============ Lifecycle ============
onMounted(() => {
  fetchImages();
  fetchStats();
  fetchStatus();
  pollInterval = window.setInterval(() => {
    fetchStatus();
    if (activeTab.value === "label") fetchStats();
  }, 3000);

  window.addEventListener("keydown", handleKeydown);
});

onUnmounted(() => {
  if (pollInterval) clearInterval(pollInterval);
  window.removeEventListener("keydown", handleKeydown);
});
</script>

<template>
  <div class="app">
    <header>
      <h1>🌌 SkyClf</h1>
      <div class="stats">
        <span class="stat">📷 {{ images.length }} images</span>
        <span class="stat">🏷️ {{ labeledCount }} labeled</span>
      </div>
    </header>

    <!-- Tabs -->
    <div class="tabs">
      <button
        :class="{ active: activeTab === 'label' }"
        @click="
          activeTab = 'label';
          fetchImages();
        "
      >
        🏷️ Label Images
      </button>
      <button
        :class="{ active: activeTab === 'train' }"
        @click="activeTab = 'train'"
      >
        🚀 Training
      </button>
    </div>

    <!-- Label Tab -->
    <div v-if="activeTab === 'label'" class="tab-content">
      <div class="label-controls">
        <label class="checkbox">
          <input
            type="checkbox"
            v-model="showUnlabeledOnly"
            @change="
              fetchImages();
              currentIndex = 0;
            "
          />
          Show unlabeled only
        </label>
        <button class="btn-small" @click="fetchImages()">🔄 Refresh</button>
      </div>

      <div v-if="images.length === 0" class="empty-state">
        <p>
          {{
            showUnlabeledOnly ? "🎉 All images labeled!" : "📷 No images yet"
          }}
        </p>
        <p class="hint">
          Images are fetched automatically from your AllSky camera
        </p>
      </div>

      <div v-else class="labeling-view">
        <!-- Image Display -->
        <div class="image-container">
          <img
            v-if="currentImage"
            :src="`/images/${currentImage.id}.jpg`"
            :alt="currentImage.id"
            class="sky-image"
          />
          <div class="image-nav">
            <button @click="prevImage" :disabled="!hasPrev" class="nav-btn">
              ◀
            </button>
            <span class="image-counter"
              >{{ currentIndex + 1 }} / {{ images.length }}</span
            >
            <button @click="nextImage" :disabled="!hasNext" class="nav-btn">
              ▶
            </button>
          </div>
        </div>

        <!-- Current Label -->
        <div v-if="currentImage?.skystate" class="current-label">
          Current: <strong>{{ currentImage.skystate }}</strong>
          <span v-if="currentImage.meteor">🌠 Meteor</span>
        </div>

        <!-- Label Buttons -->
        <div class="label-buttons">
          <button
            v-for="opt in skystateOptions"
            :key="opt.value"
            @click="setLabel(opt.value)"
            :disabled="labeling"
            :class="{ active: currentImage?.skystate === opt.value }"
            class="label-btn"
          >
            {{ opt.label }}
            <span class="key-hint">[{{ opt.key }}]</span>
          </button>
        </div>

        <!-- Meteor Toggle -->
        <div class="meteor-section">
          <button
            @click="setLabel(currentImage?.skystate || 'unknown', true)"
            :disabled="labeling || !currentImage"
            class="meteor-btn"
          >
            🌠 Mark as Meteor [M]
          </button>
        </div>

        <!-- Keyboard Hints -->
        <div class="keyboard-hints">
          <span>⌨️ Keys: 1-5 = Label • ←/→ or A/D = Navigate • M = Meteor</span>
        </div>
      </div>
    </div>

    <!-- Train Tab -->
    <div v-if="activeTab === 'train'" class="tab-content">
      <div class="card">
        <h2>Training Configuration</h2>

        <div v-if="labeledCount < 10" class="warning">
          ⚠️ You need at least 10 labeled images to train. Currently:
          {{ labeledCount }}
        </div>

        <div v-if="!status.running" class="config">
          <div class="field">
            <label>Epochs</label>
            <input type="number" v-model="epochs" min="1" max="1000" />
          </div>
          <div class="field">
            <label>Batch Size</label>
            <input type="number" v-model="batchSize" min="1" max="256" />
          </div>
          <div class="field">
            <label>Learning Rate</label>
            <input type="text" v-model="learningRate" />
          </div>
        </div>

        <div class="actions">
          <button
            v-if="!status.running"
            @click="startTraining"
            :disabled="loading || labeledCount < 10"
            class="btn-primary"
          >
            {{ loading ? "Starting..." : "🚀 Start Training" }}
          </button>
          <button
            v-else
            @click="stopTraining"
            :disabled="loading"
            class="btn-danger"
          >
            {{ loading ? "Stopping..." : "⏹️ Stop Training" }}
          </button>
        </div>

        <div v-if="error" class="error">{{ error }}</div>

        <div v-if="status.running" class="status running">
          <p>⏳ Training in progress...</p>
          <p v-if="status.started_at">
            Started: {{ new Date(status.started_at).toLocaleString() }}
          </p>
        </div>

        <div
          v-else-if="status.exit_code !== undefined && status.exit_code !== 0"
          class="status failed"
        >
          <p>❌ Training failed (exit code: {{ status.exit_code }})</p>
          <p v-if="status.error">{{ status.error }}</p>
        </div>

        <div v-else-if="status.exit_code === 0" class="status success">
          <p>✅ Training completed successfully!</p>
        </div>

        <div v-if="status.logs" class="logs">
          <h3>Logs</h3>
          <pre>{{ status.logs }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.app {
  max-width: 800px;
  margin: 0 auto;
  padding: 1rem;
  font-family: system-ui, -apple-system, sans-serif;
  color: #eee;
}

header {
  text-align: center;
  margin-bottom: 1.5rem;
}

header h1 {
  margin: 0 0 0.5rem 0;
}

.stats {
  display: flex;
  justify-content: center;
  gap: 1.5rem;
}

.stat {
  background: #1a1a2e;
  padding: 0.5rem 1rem;
  border-radius: 20px;
  font-size: 0.875rem;
}

/* Tabs */
.tabs {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 1rem;
}

.tabs button {
  flex: 1;
  padding: 0.75rem;
  border: none;
  border-radius: 8px;
  background: #1a1a2e;
  color: #888;
  font-size: 1rem;
  cursor: pointer;
  transition: all 0.2s;
}

.tabs button.active {
  background: #4f46e5;
  color: white;
}

.tabs button:hover:not(.active) {
  background: #252542;
}

/* Label Controls */
.label-controls {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}

.checkbox {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
}

.checkbox input {
  width: 18px;
  height: 18px;
}

.btn-small {
  padding: 0.5rem 1rem;
  border: none;
  border-radius: 6px;
  background: #333;
  color: #fff;
  cursor: pointer;
}

.btn-small:hover {
  background: #444;
}

/* Empty State */
.empty-state {
  text-align: center;
  padding: 3rem;
  background: #1a1a2e;
  border-radius: 12px;
}

.empty-state p {
  margin: 0.5rem 0;
}

.hint {
  color: #666;
  font-size: 0.875rem;
}

/* Labeling View */
.labeling-view {
  background: #1a1a2e;
  border-radius: 12px;
  padding: 1rem;
}

.image-container {
  position: relative;
  margin-bottom: 1rem;
}

.sky-image {
  width: 100%;
  height: auto;
  max-height: 400px;
  object-fit: contain;
  border-radius: 8px;
  background: #000;
}

.image-nav {
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 1rem;
  margin-top: 0.75rem;
}

.nav-btn {
  padding: 0.5rem 1rem;
  border: none;
  border-radius: 6px;
  background: #333;
  color: #fff;
  font-size: 1.25rem;
  cursor: pointer;
}

.nav-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.nav-btn:hover:not(:disabled) {
  background: #444;
}

.image-counter {
  color: #888;
  font-size: 0.875rem;
}

.current-label {
  text-align: center;
  padding: 0.5rem;
  background: #252542;
  border-radius: 6px;
  margin-bottom: 1rem;
}

/* Label Buttons */
.label-buttons {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 0.5rem;
  margin-bottom: 1rem;
}

.label-btn {
  padding: 0.75rem;
  border: 2px solid #333;
  border-radius: 8px;
  background: #0f0f1a;
  color: #fff;
  font-size: 0.9rem;
  cursor: pointer;
  transition: all 0.2s;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.25rem;
}

.label-btn:hover:not(:disabled) {
  border-color: #4f46e5;
  background: #1a1a3a;
}

.label-btn.active {
  border-color: #22c55e;
  background: #14532d;
}

.label-btn:disabled {
  opacity: 0.5;
}

.key-hint {
  font-size: 0.7rem;
  color: #666;
}

/* Meteor */
.meteor-section {
  margin-bottom: 1rem;
}

.meteor-btn {
  width: 100%;
  padding: 0.75rem;
  border: 2px dashed #f59e0b;
  border-radius: 8px;
  background: transparent;
  color: #f59e0b;
  font-size: 1rem;
  cursor: pointer;
}

.meteor-btn:hover:not(:disabled) {
  background: rgba(245, 158, 11, 0.1);
}

.meteor-btn:disabled {
  opacity: 0.5;
}

/* Keyboard Hints */
.keyboard-hints {
  text-align: center;
  color: #666;
  font-size: 0.75rem;
}

/* Card & Training */
.card {
  background: #1a1a2e;
  border-radius: 12px;
  padding: 1.5rem;
}

h2 {
  margin-top: 0;
  margin-bottom: 1rem;
  color: #eee;
}

.warning {
  background: #78350f;
  border-left: 4px solid #f59e0b;
  padding: 0.75rem 1rem;
  border-radius: 0 6px 6px 0;
  margin-bottom: 1rem;
}

.config {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  margin-bottom: 1.5rem;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.field label {
  font-size: 0.875rem;
  color: #aaa;
}

.field input {
  padding: 0.5rem;
  border: 1px solid #333;
  border-radius: 6px;
  background: #0f0f1a;
  color: #fff;
  font-size: 1rem;
}

.actions {
  margin-bottom: 1rem;
}

button.btn-primary,
button.btn-danger {
  width: 100%;
  padding: 0.75rem 1.5rem;
  border: none;
  border-radius: 8px;
  font-size: 1rem;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.2s;
}

button:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-primary {
  background: #4f46e5;
  color: white;
}

.btn-primary:hover:not(:disabled) {
  background: #4338ca;
}

.btn-danger {
  background: #dc2626;
  color: white;
}

.btn-danger:hover:not(:disabled) {
  background: #b91c1c;
}

.error {
  background: #7f1d1d;
  color: #fecaca;
  padding: 0.75rem;
  border-radius: 6px;
  margin-bottom: 1rem;
}

.status {
  padding: 1rem;
  border-radius: 8px;
  margin-bottom: 1rem;
}

.status.running {
  background: #1e3a5f;
  border-left: 4px solid #3b82f6;
}

.status.success {
  background: #14532d;
  border-left: 4px solid #22c55e;
}

.status.failed {
  background: #7f1d1d;
  border-left: 4px solid #ef4444;
}

.status p {
  margin: 0.25rem 0;
}

.logs {
  margin-top: 1rem;
}

.logs h3 {
  margin: 0 0 0.5rem 0;
  font-size: 0.875rem;
  color: #aaa;
}

.logs pre {
  background: #0f0f1a;
  padding: 1rem;
  border-radius: 6px;
  overflow-x: auto;
  font-size: 0.75rem;
  max-height: 300px;
  overflow-y: auto;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
}

.tab-content {
  min-height: 400px;
}
</style>
