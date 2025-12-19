<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from "vue";
import logoSvg from "./assets/SkyClfLogo.svg";

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

interface ModelInfo {
  active: string | null;
  path?: string;
}

interface Prediction {
  skystate: string;
  confidence?: number;
  probs?: Record<string, number>;
  task?: string;
  model_version?: string;
  model_path?: string;
}

interface LatestPayload {
  status: string;
  timestamp: string;
  image?: {
    id: string;
    sha256: string;
    fetched_at: string;
    url: string;
    latest_url: string;
  };
  label?: {
    skystate: string;
    meteor?: boolean | null;
    labeled_at?: string | null;
  } | null;
  prediction?: Prediction | null;
}

interface UploadPredictionResponse {
  filename?: string;
  size?: number;
  prediction: Prediction;
}

// ============ State ============
const activeTab = ref<"label" | "train" | "classify">("label");

// Labeling state
const images = ref<ImageItem[]>([]);
const currentIndex = ref(0);
const showUnlabeledOnly = ref(true);
const labeledCount = ref(0);
const totalCount = ref(0);
const unlabeledCount = ref(0);
const labeledByClass = ref<Record<string, number>>({});
const labeling = ref(false);
const clearingLabels = ref(false);
const clearMessage = ref("");

// Training state
const status = ref<TrainStatus>({ running: false });
const loading = ref(false);
const error = ref("");
const epochs = ref(10);
const batchSize = ref(16);
const learningRate = ref("0.001");
const fromScratch = ref(true);
const modelInfo = ref<ModelInfo>({ active: null });
const modelVersions = ref<any[]>([]);
const selectedModelVersion = ref<string | null>(null);
const switchingModel = ref(false);
const modelActionMessage = ref("");
const lastReloadedRunId = ref<string | null>(null);

// Classification state
const classifyLoading = ref(false);
const classifyError = ref("");
const latestClassification = ref<LatestPayload | null>(null);
const uploadFile = ref<File | null>(null);
const uploadPreview = ref<string | null>(null);
const uploadFileName = ref("");
const uploadPrediction = ref<Prediction | null>(null);
const uploadError = ref("");
const uploadLoading = ref(false);

// Performance: avoid loading thousands of images unless requested
const loadAllImages = ref(false);
const imageLimit = 400;

let pollInterval: number | null = null;

// ============ Computed ============
const currentImage = computed(() => images.value[currentIndex.value] || null);
const hasNext = computed(() => currentIndex.value < images.value.length - 1);
const hasPrev = computed(() => currentIndex.value > 0);
const progress = computed(() => {
  const labeled = labeledCount.value;
  const unlabeled = unlabeledCount.value || images.value.length;
  const total = labeled + unlabeled;
  if (total === 0) return 100;
  return Math.round((labeled / total) * 100);
});

const classBreakdown = computed(() =>
  skystateOptions.map((opt) => ({
    ...opt,
    count: labeledByClass.value[opt.value] || 0,
  }))
);

const skystateOptions = [
  {
    value: "clear",
    label: "Clear",
    icon: "mdi-weather-sunny",
    color: "#fbbf24",
    key: "1",
  },
  {
    value: "light_clouds",
    label: "Light Clouds",
    icon: "mdi-weather-partly-cloudy",
    color: "#60a5fa",
    key: "2",
  },
  {
    value: "heavy_clouds",
    label: "Heavy Clouds",
    icon: "mdi-weather-cloudy",
    color: "#6b7280",
    key: "3",
  },
  {
    value: "precipitation",
    label: "Precipitation",
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

// ============ Labeling Functions ============
async function fetchImages() {
  try {
    const params = new URLSearchParams();
    if (!loadAllImages.value) {
      params.set("limit", String(imageLimit));
    }
    if (showUnlabeledOnly.value) params.set("unlabeled", "1");

    const query = params.toString();
    const res = await fetch(
      `/api/dataset/images${query ? `?${query}` : ""}`
    );
    if (res.ok) {
      const data = await res.json();
      images.value = data.items || [];
      totalCount.value = data.count || 0;
      // Keep index valid so the main image and timeline don't disappear after refreshes
      if (currentIndex.value >= images.value.length) {
        currentIndex.value = images.value.length > 0 ? images.value.length - 1 : 0;
      }
    }
  } catch (e) {
    console.error("Failed to fetch images:", e);
  }
}

async function fetchStats() {
  try {
    const res = await fetch("/api/dataset/stats");
    if (res.ok) {
      const data = await res.json();
      labeledCount.value = data.labeled ?? 0;
      totalCount.value = data.total ?? 0;
      unlabeledCount.value = data.unlabeled ?? 0;
      labeledByClass.value = data.by_class ?? {};
    } else {
      // Fallback to old behavior if stats route is unavailable
      const alt = await fetch("/api/dataset/images");
      if (alt.ok) {
        const data = await alt.json();
        const items = data.items || [];
        labeledCount.value = items.filter((i: ImageItem) => i.skystate).length;
        unlabeledCount.value = Math.max((data.count || 0) - labeledCount.value, 0);
        labeledByClass.value = {};
      }
    }
  } catch (e) {
    console.error("Failed to fetch stats:", e);
  }
}

async function fetchModelInfo() {
  try {
    const res = await fetch("/api/models");
    if (res.ok) {
      modelInfo.value = await res.json();
    }
  } catch (e) {
    console.error("Failed to fetch model info:", e);
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
      if (showUnlabeledOnly.value) {
        images.value.splice(currentIndex.value, 1);
        if (
          currentIndex.value >= images.value.length &&
          images.value.length > 0
        ) {
          currentIndex.value = images.value.length - 1;
        }
      } else {
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

function setCurrentIndex(idx: number) {
  if (idx < 0 || idx >= images.value.length) return;
  currentIndex.value = idx;
}

async function clearAllLabels() {
  if (clearingLabels.value) return;
  const confirmClear = window.confirm(
    "Remove ALL labels? This cannot be undone. Images stay, labels are cleared."
  );
  if (!confirmClear) return;

  clearingLabels.value = true;
  clearMessage.value = "";
  try {
    const res = await fetch("/api/labels/reset?confirm=yes", { method: "POST" });
    const data = res.ok ? await res.json() : null;
    if (!res.ok) {
      clearMessage.value = data?.error || "Failed to remove labels";
    } else {
      clearMessage.value = data?.message || "All labels removed";
      await Promise.all([fetchStats(), fetchImages()]);
      currentIndex.value = 0;
    }
  } catch (e) {
    clearMessage.value = "Network error";
  } finally {
    clearingLabels.value = false;
  }
}

function handleKeydown(e: KeyboardEvent) {
  if ((e.target as HTMLElement).tagName === "INPUT") return;

  if (activeTab.value !== "label" || !currentImage.value) return;

  const keyIndex = parseInt(e.key) - 1;
  if (keyIndex >= 0 && keyIndex < skystateOptions.length) {
    const opt = skystateOptions[keyIndex];
    if (opt) setLabel(opt.value);
    return;
  }

  if (e.key === "ArrowRight" || e.key === "d" || e.key === " ") {
    e.preventDefault();
    nextImage();
  }
  if (e.key === "ArrowLeft" || e.key === "a") {
    e.preventDefault();
    prevImage();
  }

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
        from_scratch: fromScratch.value,
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

async function reloadModels(version?: string) {
  try {
    const q = version ? `?version=${encodeURIComponent(version)}` : "";
    await fetch(`/api/models/reload${q}`, { method: "POST" });
    await fetchModelInfo();
  } catch (e) {
    console.error("Failed to reload models:", e);
  }
}

async function fetchModelVersions() {
  try {
    const res = await fetch("/api/models/list");
    if (res.ok) {
      modelVersions.value = await res.json();
    }
  } catch (e) {
    modelVersions.value = [];
  }
}

function pickFormatsForVersion(version: string | null) {
  if (!version) return [];
  const item = modelVersions.value.find((m) => m.version === version);
  if (!item) return [];
  const formats: string[] = [];
  if (item["model.onnx"]) formats.push("model.onnx");
  if (item["model.pt"]) formats.push("model.pt");
  return formats;
}

function syncSelectedModel() {
  const list = modelVersions.value;
  const active = modelInfo.value.active;
  let target = selectedModelVersion.value;
  if (!target || !list.some((m) => m.version === target)) {
    if (active && list.some((m) => m.version === active)) {
      target = active;
    } else if (list.length) {
      target = list[0].version;
    } else {
      target = null;
    }
  }
  selectedModelVersion.value = target;
}

const selectedVersion = computed(() => {
  if (!selectedModelVersion.value) return null;
  return (
    modelVersions.value.find((m) => m.version === selectedModelVersion.value) ||
    null
  );
});

const selectedFormats = computed(() => pickFormatsForVersion(selectedModelVersion.value));

async function useSelectedModel() {
  if (!selectedModelVersion.value) return;
  switchingModel.value = true;
  modelActionMessage.value = "";
  try {
    await reloadModels(selectedModelVersion.value);
    await fetchModelVersions();
    modelActionMessage.value = `Model switched to ${selectedModelVersion.value}`;
  } catch (e) {
    console.error("Failed to switch model", e);
    modelActionMessage.value = "Could not switch model";
  } finally {
    switchingModel.value = false;
  }
}

// ============ Classification Functions ============
async function classifyLatest() {
  classifyLoading.value = true;
  classifyError.value = "";
  try {
    const res = await fetch("/api/latest");
    if (!res.ok) {
      const msg = await res.text();
      throw new Error(msg || "Failed to classify latest image");
    }
    latestClassification.value = await res.json();
  } catch (e: any) {
    console.error("Failed to classify latest image", e);
    classifyError.value =
      e?.message || "Could not classify the latest captured image";
    latestClassification.value = null;
  } finally {
    classifyLoading.value = false;
  }
}

function revokeUploadPreview() {
  if (uploadPreview.value) {
    URL.revokeObjectURL(uploadPreview.value);
    uploadPreview.value = null;
  }
}

function clearUpload() {
  revokeUploadPreview();
  uploadFile.value = null;
  uploadFileName.value = "";
  uploadPrediction.value = null;
  uploadError.value = "";
}

function onUploadFileChange(e: Event) {
  const target = e.target as HTMLInputElement;
  const file = target.files?.[0] || null;
  revokeUploadPreview();
  uploadFile.value = file;
  uploadPrediction.value = null;
  uploadError.value = "";
  classifyError.value = "";

  if (file) {
    uploadPreview.value = URL.createObjectURL(file);
    uploadFileName.value = file.name;
  } else {
    uploadFileName.value = "";
  }
}

async function submitUploadClassification() {
  if (!uploadFile.value) {
    uploadError.value = "Please choose an image first.";
    return;
  }

  uploadLoading.value = true;
  uploadError.value = "";
  try {
    const form = new FormData();
    form.append("file", uploadFile.value);
    const res = await fetch("/api/classify", { method: "POST", body: form });
    if (!res.ok) {
      const msg = await res.text();
      throw new Error(msg || "Upload failed");
    }
    const data: UploadPredictionResponse = await res.json();
    uploadPrediction.value = data.prediction;
    uploadFileName.value = data.filename || uploadFile.value.name;
  } catch (e: any) {
    console.error("Upload classification failed", e);
    uploadError.value = e?.message || "Could not classify the uploaded file";
    uploadPrediction.value = null;
  } finally {
    uploadLoading.value = false;
  }
}


// Update model list on mount and after training
onMounted(() => {
  fetchModelVersions();
});

watch([status, modelInfo], () => {
  fetchModelVersions();
  syncSelectedModel();
});

watch(modelVersions, () => {
  syncSelectedModel();
});

watch(status, (newVal) => {
  const runId = newVal?.started_at || null;
  const justFinished =
    newVal &&
    !newVal.running &&
    newVal.exit_code === 0 &&
    runId !== null &&
    lastReloadedRunId.value !== runId;
  if (justFinished) {
    lastReloadedRunId.value = runId;
    reloadModels();
    fetchModelVersions();
  }
  if (newVal?.running) {
    lastReloadedRunId.value = null;
  }
});

watch(activeTab, (tab) => {
  if (tab === "classify" && !latestClassification.value && !classifyLoading.value) {
    classifyLatest();
  }
});

// ============ Lifecycle ============
onMounted(() => {
  fetchImages();
  fetchStats();
  fetchStatus();
  fetchModelInfo();
  pollInterval = window.setInterval(() => {
    fetchStatus();
    fetchModelInfo();
    if (activeTab.value === "label") fetchStats();
  }, 3000);

  window.addEventListener("keydown", handleKeydown);
});

onUnmounted(() => {
  if (pollInterval) clearInterval(pollInterval);
  window.removeEventListener("keydown", handleKeydown);
  clearUpload();
});
</script>

<template>
  <div class="app">
    <!-- Sidebar -->
    <aside class="sidebar">
      <div class="logo">
        <img :src="logoSvg" alt="SkyClf" class="logo-img" />
        <span class="logo-text">SkyClf</span>
      </div>

      <nav class="nav">
        <button
          :class="['nav-item', { active: activeTab === 'label' }]"
          @click="
            activeTab = 'label';
            fetchImages();
          "
        >
          <span class="mdi mdi-tag-multiple"></span>
          <span>Label</span>
        </button>
        <button
          :class="['nav-item', { active: activeTab === 'train' }]"
          @click="activeTab = 'train'"
        >
          <span class="mdi mdi-brain"></span>
          <span>Train</span>
        </button>
        <button
          :class="['nav-item', { active: activeTab === 'classify' }]"
          @click="
            activeTab = 'classify';
            classifyLatest();
          "
        >
          <span class="mdi mdi-camera-iris"></span>
          <span>Classify</span>
        </button>
      </nav>

      <div class="sidebar-stats">
        <div class="stat-item">
          <span class="mdi mdi-image-multiple"></span>
          <div class="stat-content">
            <span class="stat-value">{{
              unlabeledCount || images.length
            }}</span>
            <span class="stat-label">Unlabeled</span>
          </div>
        </div>
        <div class="stat-item">
          <span class="mdi mdi-check-circle"></span>
          <div class="stat-content">
            <span class="stat-value">{{ labeledCount }}</span>
            <span class="stat-label">Labeled</span>
          </div>
        </div>
        <div class="stat-item">
          <span class="mdi mdi-cube"></span>
          <div class="stat-content">
            <span class="stat-value">{{ modelInfo.active || "None" }}</span>
            <span class="stat-label">Model</span>
          </div>
        </div>

        <div class="class-breakdown">
          <div class="class-breakdown-header">
            <span>By class</span>
            <span class="muted">{{ labeledCount }} labeled</span>
          </div>
          <div class="class-chip-list">
            <span
              v-for="cls in classBreakdown"
              :key="cls.value"
              class="class-chip"
              :style="{ '--class-color': cls.color }"
            >
              <span class="chip-dot"></span>
              <span class="chip-label">{{ cls.label }}</span>
              <span class="chip-count">{{ cls.count }}</span>
            </span>
          </div>
        </div>
      </div>

      <div class="progress-section">
        <div class="progress-header">
          <span>Progress</span>
          <span>{{ progress }}%</span>
        </div>
        <div class="progress-bar">
          <div class="progress-fill" :style="{ width: progress + '%' }"></div>
        </div>
      </div>
    </aside>

    <!-- Main Content -->
    <main class="main">
      <!-- Label View -->
      <div v-if="activeTab === 'label'" class="label-view">
        <!-- Toolbar -->
        <div class="toolbar">
          <div class="toolbar-left">
            <h1>Label Images</h1>
          </div>
          <div class="toolbar-right">
            <div class="danger-inline">
              <button
                class="btn-danger ghost"
                @click="clearAllLabels"
                :disabled="clearingLabels"
                title="Remove all labels (images remain)"
              >
                <span class="mdi mdi-alert"></span>
                {{ clearingLabels ? "Clearing..." : "Remove all labels" }}
              </button>
              <span class="danger-hint">Clears every label; images are kept.</span>
            </div>
            <label class="toggle">
              <input
                type="checkbox"
                v-model="showUnlabeledOnly"
                @change="
                  fetchImages();
                  currentIndex = 0;
                "
              />
              <span class="toggle-slider"></span>
              <span class="toggle-label">Unlabeled only</span>
            </label>
            <label class="toggle">
              <input
                type="checkbox"
                v-model="loadAllImages"
                @change="
                  fetchImages();
                  currentIndex = 0;
                "
              />
              <span class="toggle-slider"></span>
              <span class="toggle-label">Load all (slower)</span>
            </label>
            <button class="icon-btn" @click="fetchImages()" title="Refresh">
              <span class="mdi mdi-refresh"></span>
            </button>
          </div>
        </div>

        <p v-if="clearMessage" class="clear-message">{{ clearMessage }}</p>

        <!-- Empty State -->
        <div v-if="images.length === 0" class="empty-state">
          <span class="mdi mdi-party-popper empty-icon"></span>
          <h2>{{ showUnlabeledOnly ? "All Done!" : "No Images" }}</h2>
          <p>
            {{
              showUnlabeledOnly
                ? "All images have been labeled."
                : "Images will appear here as they are captured."
            }}
          </p>
        </div>

        <!-- Labeling Interface -->
        <div v-else class="labeling-interface">
          <!-- Image Panel -->
          <div class="image-panel">
            <div class="image-wrapper">
              <img
                v-if="currentImage"
                :src="`/images/${currentImage.id}.jpg`"
                :alt="currentImage.id"
                class="main-image"
              />

              <!-- Navigation Overlay -->
              <button
                class="nav-overlay nav-prev"
                @click="prevImage"
                :disabled="!hasPrev"
              >
                <span class="mdi mdi-chevron-left"></span>
              </button>
              <button
                class="nav-overlay nav-next"
                @click="nextImage"
                :disabled="!hasNext"
              >
                <span class="mdi mdi-chevron-right"></span>
              </button>

              <!-- Current Label Badge -->
              <div v-if="currentImage?.skystate" class="current-badge">
                <span
                  class="mdi"
                  :class="
                    skystateOptions.find(
                      (o) => o.value === currentImage?.skystate
                    )?.icon
                  "
                ></span>
                {{ currentImage.skystate }}
                <span v-if="currentImage.meteor" class="meteor-badge">
                  <span class="mdi mdi-star-shooting"></span>
                </span>
              </div>
            </div>

            <!-- Image Counter -->
            <div class="image-counter">
              <button
                class="counter-btn"
                @click="prevImage"
                :disabled="!hasPrev"
              >
                <span class="mdi mdi-arrow-left"></span>
              </button>
              <span class="counter-text"
                >{{ currentIndex + 1 }} of {{ images.length }}</span
              >
              <button
                class="counter-btn"
                @click="nextImage"
                :disabled="!hasNext"
              >
                <span class="mdi mdi-arrow-right"></span>
              </button>
            </div>

            <div class="image-timeline">
              <div class="timeline-header">
                <span class="mdi mdi-filmstrip-box-multiple"></span>
                <span>Timeline</span>
                <span class="timeline-count">{{ images.length }} images</span>
              </div>
              <div class="timeline-track">
                <button
                  v-for="(img, idx) in images"
                  :key="img.id"
                  class="timeline-thumb"
                  :class="{ active: idx === currentIndex }"
                  @click="setCurrentIndex(idx)"
                  :title="`Jump to ${img.id}`"
                >
                  <img :src="`/images/${img.id}.jpg`" :alt="img.id" />
                  <span class="thumb-label">
                    {{
                      new Date(img.fetched_at).toLocaleTimeString([], {
                        hour: "2-digit",
                        minute: "2-digit",
                      })
                    }}
                  </span>
                </button>
              </div>
            </div>
          </div>

          <!-- Controls Panel -->
          <div class="controls-panel">
            <h3>Sky State</h3>
            <div class="label-grid">
              <button
                v-for="opt in skystateOptions"
                :key="opt.value"
                @click="setLabel(opt.value)"
                :disabled="labeling"
                :class="[
                  'label-card',
                  { active: currentImage?.skystate === opt.value },
                ]"
                :style="{ '--accent-color': opt.color }"
              >
                <span class="mdi" :class="opt.icon"></span>
                <span class="label-name">{{ opt.label }}</span>
                <span class="label-key">{{ opt.key }}</span>
              </button>
            </div>

            <div class="divider"></div>

            <h3>Special</h3>
            <button
              @click="setLabel(currentImage?.skystate || 'unknown', true)"
              :disabled="labeling || !currentImage"
              class="meteor-card"
            >
              <span class="mdi mdi-star-shooting"></span>
              <span>Meteor Detected</span>
              <span class="label-key">M</span>
            </button>

            <div class="shortcuts">
              <div class="shortcut">
                <kbd>1-5</kbd>
                <span>Select label</span>
              </div>
              <div class="shortcut">
                <kbd>←</kbd><kbd>→</kbd>
                <span>Navigate</span>
              </div>
              <div class="shortcut">
                <kbd>Space</kbd>
                <span>Next image</span>
              </div>
              <div class="shortcut">
                <kbd>M</kbd>
                <span>Mark meteor</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Classify View -->
      <div v-if="activeTab === 'classify'" class="classify-view">
        <div class="toolbar">
          <div class="toolbar-left">
            <h1>Classify</h1>
          </div>
          <div class="toolbar-right">
            <button
              class="btn-primary ghost"
              @click="classifyLatest"
              :disabled="classifyLoading"
            >
              <span class="mdi mdi-camera-iris"></span>
              {{ classifyLoading ? "Classifying..." : "Classify now" }}
            </button>
          </div>
        </div>

        <div class="classify-content">
          <div class="classify-card">
            <div class="classify-card-header">
              <div>
                <h3>Latest capture</h3>
                <p class="muted">
                  Run inference on the newest image pulled from the AllSky feed.
                </p>
              </div>
              <span class="pill">Live</span>
            </div>

            <p v-if="classifyError" class="error-message small">
              {{ classifyError }}
            </p>

            <div v-if="classifyLoading" class="loading-row">
              <span class="mdi mdi-loading mdi-spin"></span>
              <span>Running model...</span>
            </div>

            <div v-if="latestClassification?.image" class="latest-result">
              <div class="latest-image">
                <img
                  :src="`${latestClassification.image.latest_url}?ts=${latestClassification.timestamp}`"
                  :alt="latestClassification.image.id"
                />
              </div>
              <div class="latest-meta">
                <h4>{{ latestClassification.image.id }}</h4>
                <p class="muted">
                  {{
                    new Date(
                      latestClassification.image.fetched_at
                    ).toLocaleString()
                  }}
                </p>
                <div
                  class="prediction-badge"
                  v-if="latestClassification.prediction"
                >
                  <span class="mdi mdi-brain"></span>
                  <span class="pred-label">
                    {{ latestClassification.prediction.skystate }}
                  </span>
                  <span
                    class="pred-confidence"
                    v-if="
                      latestClassification.prediction.confidence !== undefined
                    "
                  >
                    {{
                      (
                        latestClassification.prediction.confidence * 100
                      ).toFixed(1)
                    }}%
                  </span>
                </div>
                <div class="prediction-badge subtle" v-else>
                  <span class="mdi mdi-alert"></span>
                  <span>No prediction available</span>
                </div>
                <div
                  class="prob-row"
                  v-if="latestClassification.prediction?.probs"
                >
                  <div
                    v-for="(val, key) in latestClassification.prediction.probs"
                    :key="key"
                    class="prob-item"
                  >
                    <span class="muted">{{ key }}</span>
                    <div class="prob-bar">
                      <div
                        class="prob-fill"
                        :style="{ width: Math.round(val * 100) + '%' }"
                      ></div>
                    </div>
                    <span class="prob-value">{{ Math.round(val * 100) }}%</span>
                  </div>
                </div>
              </div>
            </div>

            <div v-else-if="!classifyLoading" class="empty-state compact">
              <span class="mdi mdi-camera-off empty-icon"></span>
              <h2>No image yet</h2>
              <p>Waiting for an image to classify.</p>
            </div>
          </div>

          <div class="classify-card">
            <div class="classify-card-header">
              <div>
                <h3>Upload &amp; test</h3>
                <p class="muted">
                  Send any JPG/PNG and get the model prediction back instantly.
                </p>
              </div>
            </div>

            <label class="upload-drop">
              <input type="file" accept="image/*" @change="onUploadFileChange" />
              <div>
                <span class="mdi mdi-upload"></span>
                <span>{{ uploadFileName || "Choose an image" }}</span>
              </div>
            </label>

            <div v-if="uploadPreview" class="upload-preview">
              <img :src="uploadPreview" :alt="uploadFileName" />
              <button
                class="icon-btn"
                @click="clearUpload"
                title="Clear selection"
              >
                <span class="mdi mdi-close"></span>
              </button>
            </div>

            <p v-if="uploadError" class="error-message small">
              {{ uploadError }}
            </p>

            <div class="upload-actions">
              <button
                class="btn-primary"
                @click="submitUploadClassification"
                :disabled="uploadLoading"
              >
                <span class="mdi mdi-ray-start-arrow"></span>
                {{ uploadLoading ? "Classifying..." : "Classify upload" }}
              </button>
            </div>

            <div v-if="uploadPrediction" class="upload-result">
              <div class="prediction-badge">
                <span class="mdi mdi-check"></span>
                <span class="pred-label">{{ uploadPrediction.skystate }}</span>
                <span
                  class="pred-confidence"
                  v-if="uploadPrediction.confidence !== undefined"
                >
                  {{ (uploadPrediction.confidence * 100).toFixed(1) }}%
                </span>
              </div>
              <div class="prob-row" v-if="uploadPrediction.probs">
                <div
                  v-for="(val, key) in uploadPrediction.probs"
                  :key="key"
                  class="prob-item"
                >
                  <span class="muted">{{ key }}</span>
                  <div class="prob-bar">
                    <div
                      class="prob-fill"
                      :style="{ width: Math.round(val * 100) + '%' }"
                    ></div>
                  </div>
                  <span class="prob-value">{{ Math.round(val * 100) }}%</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Train View -->
      <div v-if="activeTab === 'train'" class="train-view">
        <div class="toolbar">
          <div class="toolbar-left">
            <h1>Train Model</h1>
          </div>
          <div class="toolbar-right">
            <button
              class="icon-btn"
              @click="reloadModels()"
              title="Reload Models"
            >
              <span class="mdi mdi-reload"></span>
            </button>
          </div>
        </div>

        <div class="train-content">
          <!-- Status Cards -->
          <div class="status-cards">
            <div class="status-card">
              <span class="mdi mdi-tag-check"></span>
              <div class="status-info">
                <span class="status-value">{{ labeledCount }}</span>
                <span class="status-label">Labeled Images</span>
              </div>
            </div>
            <div class="status-card">
              <span class="mdi mdi-cube-outline"></span>
              <div class="status-info">
                <span class="status-value">{{
                  modelInfo.active || "None"
                }}</span>
                <span class="status-label">Current Model</span>
              </div>
            </div>
            <div
              class="status-card"
              :class="{ 'status-running': status.running }"
            >
              <span
                class="mdi"
                :class="
                  status.running ? 'mdi-loading mdi-spin' : 'mdi-check-circle'
                "
              ></span>
              <div class="status-info">
                <span class="status-value">{{
                  status.running ? "Training" : "Idle"
                }}</span>
                <span class="status-label">Status</span>
              </div>
            </div>
          </div>

          <!-- Warning -->
          <div v-if="labeledCount < 10" class="warning-banner">
            <span class="mdi mdi-alert"></span>
            <div>
              <strong>Not enough data</strong>
              <p>
                You need at least 10 labeled images to train. Currently:
                {{ labeledCount }}
              </p>
            </div>
          </div>

          <!-- Config Form -->
          <div class="config-section" v-if="!status.running">
            <h2>Configuration</h2>
            <div class="config-grid">
              <div class="config-field">
                <label>
                  <span class="mdi mdi-repeat"></span>
                  Epochs
                </label>
                <input type="number" v-model="epochs" min="1" max="1000" />
                <span class="field-hint">Training iterations</span>
              </div>
              <div class="config-field">
                <label>
                  <span class="mdi mdi-package-variant"></span>
                  Batch Size
                </label>
                <input type="number" v-model="batchSize" min="1" max="256" />
                <span class="field-hint">Images per batch</span>
              </div>
              <div class="config-field">
                <label>
                  <span class="mdi mdi-speedometer"></span>
                  Learning Rate
                </label>
                <input type="text" v-model="learningRate" />
                <span class="field-hint">Step size (e.g. 0.001)</span>
              </div>
            </div>

            <!-- From Scratch Option -->
            <label class="scratch-toggle">
              <input type="checkbox" v-model="fromScratch" />
              <span
                class="mdi"
                :class="fromScratch ? 'mdi-restart' : 'mdi-school'"
              ></span>
              <div class="scratch-content">
                <span class="scratch-label">{{
                  fromScratch ? "Train from scratch" : "Continue learning"
                }}</span>
                <span class="scratch-hint">{{
                  fromScratch
                    ? "Reset weights and start a brand-new model"
                    : "Fine-tune the currently active model"
                }}</span>
                <span class="scratch-mode-note">
                  {{
                    fromScratch
                      ? "Use when you want to ignore past training."
                      : "Keeps existing weights and learns from new labels."
                  }}
                </span>
              </div>
            </label>
          </div>

          <!-- Action Buttons -->
          <div class="actions">
            <button
              v-if="!status.running"
              @click="startTraining"
              :disabled="loading || labeledCount < 10"
              class="btn-primary"
            >
              <span class="mdi mdi-play"></span>
              {{
                loading
                  ? "Starting..."
                  : fromScratch
                  ? "Start (from scratch)"
                  : "Start (continue learning)"
              }}
            </button>
            <button
              v-else
              @click="stopTraining"
              :disabled="loading"
              class="btn-danger"
            >
              <span class="mdi mdi-stop"></span>
              {{ loading ? "Stopping..." : "Stop Training" }}
            </button>
          </div>

          <div class="manual-card">
            <h3>Quick training manual</h3>
            <ul class="manual-list">
              <li>
                <strong>1) Initial training</strong>: collect ~50 images per
                class, roughly balanced. Include day and night clear skies.
                Disable "Continue learning" and run Train from scratch once.
              </li>
              <li>
                <strong>2) Improve over time</strong>: label a small, balanced
                batch of new images, enable "Continue learning", then train
                again (weekly or after enough new data).
              </li>
              <li>
                <strong>3) Validation & performance</strong>: keep validation
                split = 0.2 and seed = 42 for repeatable results. Use a small
                batch size if GPU memory is limited.
              </li>
              <li>
                <strong>4) Review mistakes</strong>: after each training, reload
                the model, check wrong or very confident predictions, and
                relabel those images. This boosts accuracy more than random new
                data.
              </li>
              <li>
                <strong>5) Periodic reset</strong>: after several fine-tunes or
                big dataset changes, disable "Continue learning" and run Train
                from scratch again to prevent drift.
              </li>
            </ul>
            <p class="muted">
              Rule of thumb: few new labels → Continue learning. Big changes or
              seasonal shift → Train from scratch.
            </p>
          </div>

          <!-- Models & API -->
          <div class="download-section">
            <div class="model-card">
              <div class="model-card-header">
                <div>
                  <h3>Models & API</h3>
                  <p class="muted">
                    Switch or download a version. <a
                      href="/api/clf"
                      class="api-link"
                      target="_blank"
                      >/api/clf</a
                    >
                    returns the latest prediction JSON.
                  </p>
                </div>
                <span class="pill">
                  Active: {{ modelInfo.active || "None" }}
                </span>
              </div>

              <div class="model-controls">
                <label class="model-field">
                  <span>Version</span>
                  <select v-model="selectedModelVersion">
                    <option
                      v-for="m in modelVersions"
                      :key="m.version"
                      :value="m.version"
                    >
                      {{ m.version }}
                      <template v-if="m.created_at">
                        ({{ new Date(m.created_at).toLocaleString() }})
                      </template>
                    </option>
                    <option v-if="!modelVersions.length" disabled>
                      No models available
                    </option>
                  </select>
                </label>
                <div class="model-actions wrap">
                  <a
                    v-for="fmt in selectedFormats"
                    :key="fmt"
                    class="btn-download"
                    :href="
                      selectedVersion && selectedVersion[fmt]
                        ? selectedVersion[fmt]
                        : `/api/models/download?version=${selectedModelVersion}&file=${fmt}`
                    "
                    download
                  >
                    <span class="mdi mdi-download"></span>
                    Download {{ fmt === 'model.onnx' ? 'ONNX' : 'PyTorch' }}
                  </a>
                  <button
                    class="btn-primary ghost"
                    @click="useSelectedModel"
                    :disabled="!selectedModelVersion || switchingModel"
                  >
                    <span class="mdi mdi-rocket-launch"></span>
                    {{ switchingModel ? "Switching..." : "Use this version" }}
                  </button>
                </div>
                <p v-if="modelActionMessage" class="model-action-message">
                  {{ modelActionMessage }}
                </p>
              </div>

              <div v-if="modelVersions.length" class="model-list">
                <h3>All Versions</h3>
                <ul>
                  <li v-for="m in modelVersions" :key="m.version">
                    <div class="model-version-row">
                      <div>
                        <span class="model-version">{{ m.version }}</span>
                        <span v-if="m.created_at" class="model-created"
                          >{{ new Date(m.created_at).toLocaleString() }}</span
                        >
                      </div>
                      <div class="model-links">
                        <a
                          v-if="m['model.onnx']"
                          :href="m['model.onnx']"
                          class="btn-download btn-small"
                          download
                        >
                          <span class="mdi mdi-download"></span>ONNX
                        </a>
                        <a
                          v-if="m['model.pt']"
                          :href="m['model.pt']"
                          class="btn-download btn-small"
                          download
                        >
                          <span class="mdi mdi-download"></span>PyTorch
                        </a>
                        <span v-if="modelInfo.active === m.version" class="pill"
                          >Active</span
                        >
                        <button
                          v-else
                          class="btn-primary ghost btn-small"
                          @click="
                            selectedModelVersion = m.version;
                            useSelectedModel();
                          "
                        >
                          Use
                        </button>
                      </div>
                    </div>
                  </li>
                </ul>
              </div>
            </div>
          </div>
          <div class="info-note">
            <span class="mdi mdi-database"></span>
            Labeled images stay in <code>/data/images</code> and labels in
            <code>/data/labels/labels.db</code>. Training never deletes them, so
            you can retrain or audit later.
          </div>

          <!-- Error Message -->
          <div v-if="error" class="error-message">
            <span class="mdi mdi-alert-circle"></span>
            {{ error }}
          </div>

          <!-- Training Status -->
          <div v-if="status.running" class="training-status">
            <div class="training-header">
              <span class="mdi mdi-loading mdi-spin"></span>
              <span>Training in progress...</span>
            </div>
            <p v-if="status.started_at" class="training-time">
              Started {{ new Date(status.started_at).toLocaleString() }}
            </p>
          </div>

          <div v-else-if="status.exit_code === 0" class="success-status">
            <span class="mdi mdi-check-circle"></span>
            <span>Training completed successfully!</span>
          </div>

          <div
            v-else-if="status.exit_code !== undefined && status.exit_code !== 0"
            class="failed-status"
          >
            <span class="mdi mdi-close-circle"></span>
            <span>Training failed (exit code: {{ status.exit_code }})</span>
          </div>

          <!-- Logs -->
          <div v-if="status.logs" class="logs-section">
            <h3>
              <span class="mdi mdi-console"></span>
              Logs
            </h3>
            <pre class="logs-content">{{ status.logs }}</pre>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<style>
@import url("https://cdn.jsdelivr.net/npm/@mdi/font@7.4.47/css/materialdesignicons.min.css");

* {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

html,
body,
#app {
  height: 100%;
  width: 100%;
  overflow: auto;
}

body {
  font-family: "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
    sans-serif;
  background: #0a0a0f;
  color: #e4e4e7;
}

.app {
  display: flex;
  height: 100vh;
  width: 100vw;
}

/* Sidebar */
.sidebar {
  width: 240px;
  background: #111118;
  border-right: 1px solid #27272a;
  display: flex;
  flex-direction: column;
  padding: 1.5rem 1rem;
  flex-shrink: 0;
}

.logo {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0 0.5rem;
  margin-bottom: 2rem;
}

.logo .mdi {
  font-size: 2rem;
  color: #8b5cf6;
}

.logo-img {
  width: 36px;
  height: 36px;
  object-fit: contain;
}

.logo-text {
  font-size: 1.5rem;
  font-weight: 700;
  background: linear-gradient(135deg, #8b5cf6, #6366f1);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.nav {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin-bottom: 2rem;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.875rem 1rem;
  border: none;
  border-radius: 12px;
  background: transparent;
  color: #71717a;
  font-size: 0.9375rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.nav-item .mdi {
  font-size: 1.25rem;
}

.nav-item:hover {
  background: #18181b;
  color: #e4e4e7;
}

.nav-item.active {
  background: linear-gradient(
    135deg,
    rgba(139, 92, 246, 0.2),
    rgba(99, 102, 241, 0.2)
  );
  color: #a78bfa;
}

.sidebar-stats {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-bottom: 2rem;
}

.stat-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem;
  background: #18181b;
  border-radius: 10px;
}

.stat-item .mdi {
  font-size: 1.25rem;
  color: #71717a;
}

.stat-content {
  display: flex;
  flex-direction: column;
}

.stat-value {
  font-size: 1rem;
  font-weight: 600;
  color: #fafafa;
}

.stat-label {
  font-size: 0.75rem;
  color: #71717a;
}

.muted {
  color: #71717a;
  font-size: 0.8125rem;
}

.class-breakdown {
  padding: 0.75rem;
  background: #18181b;
  border-radius: 12px;
  border: 1px solid #27272a;
}

.class-breakdown-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 0.875rem;
  color: #e4e4e7;
  margin-bottom: 0.5rem;
}

.class-chip-list {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
}

.class-chip {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.35rem 0.5rem;
  border-radius: 10px;
  background: #111118;
  border: 1px solid #27272a;
  color: #e4e4e7;
}

.chip-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--class-color, #8b5cf6);
}

.chip-label {
  font-size: 0.8125rem;
}

.chip-count {
  font-weight: 600;
  color: #fafafa;
}

.progress-section {
  margin-top: auto;
  padding: 1rem;
  background: #18181b;
  border-radius: 12px;
}

.progress-header {
  display: flex;
  justify-content: space-between;
  font-size: 0.8125rem;
  color: #a1a1aa;
  margin-bottom: 0.5rem;
}

.progress-bar {
  height: 6px;
  background: #27272a;
  border-radius: 3px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background: linear-gradient(90deg, #8b5cf6, #6366f1);
  border-radius: 3px;
  transition: width 0.3s ease;
}

/* Main Content */
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: auto;
}

.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1.25rem 2rem;
  border-bottom: 1px solid #27272a;
  background: #111118;
}

.toolbar h1 {
  font-size: 1.5rem;
  font-weight: 600;
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 1rem;
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

.toggle-slider {
  width: 44px;
  height: 24px;
  background: #27272a;
  border-radius: 12px;
  position: relative;
  transition: background 0.2s;
}

.toggle-slider::after {
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

.toggle input:checked + .toggle-slider {
  background: #8b5cf6;
}

.toggle input:checked + .toggle-slider::after {
  transform: translateX(20px);
  background: white;
}

.toggle-label {
  font-size: 0.875rem;
  color: #a1a1aa;
}

.icon-btn {
  width: 40px;
  height: 40px;
  border: none;
  border-radius: 10px;
  background: #18181b;
  color: #a1a1aa;
  font-size: 1.25rem;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.icon-btn:hover {
  background: #27272a;
  color: #fafafa;
}

/* Label View */
.label-view {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: auto;
}

.empty-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: #71717a;
}

.empty-icon {
  font-size: 4rem;
  margin-bottom: 1rem;
  color: #8b5cf6;
}

.empty-state h2 {
  font-size: 1.5rem;
  color: #fafafa;
  margin-bottom: 0.5rem;
}

.empty-state p {
  font-size: 0.9375rem;
}

/* Labeling Interface */
.labeling-interface {
  flex: 1;
  display: grid;
  grid-template-columns: 1fr 320px;
  overflow: auto;
}

.image-panel {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 1.5rem;
  background: #0a0a0f;
  overflow-y: auto;
  min-height: 0;
}

.image-wrapper {
  flex: 1;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #000;
  border-radius: 16px;
  overflow: hidden;
  min-height: 0;
}

.main-image {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}

.nav-overlay {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 80px;
  border: none;
  background: transparent;
  color: white;
  font-size: 2.5rem;
  cursor: pointer;
  opacity: 0;
  transition: opacity 0.2s;
  display: flex;
  align-items: center;
  justify-content: center;
}

.nav-overlay:hover:not(:disabled) {
  opacity: 1;
  background: linear-gradient(90deg, rgba(0, 0, 0, 0.5), transparent);
}

.nav-prev {
  left: 0;
}

.nav-next {
  right: 0;
}

.nav-next:hover:not(:disabled) {
  background: linear-gradient(-90deg, rgba(0, 0, 0, 0.5), transparent);
}

.nav-overlay:disabled {
  cursor: default;
}

.current-badge {
  position: absolute;
  bottom: 1rem;
  left: 1rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 1rem;
  background: rgba(0, 0, 0, 0.75);
  backdrop-filter: blur(8px);
  border-radius: 8px;
  font-size: 0.875rem;
  font-weight: 500;
  text-transform: capitalize;
}

.meteor-badge {
  color: #fbbf24;
}

.image-counter {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1rem;
  padding: 1rem;
}

.counter-btn {
  width: 36px;
  height: 36px;
  border: none;
  border-radius: 8px;
  background: #18181b;
  color: #a1a1aa;
  font-size: 1.25rem;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.counter-btn:hover:not(:disabled) {
  background: #27272a;
  color: #fafafa;
}

.counter-btn:disabled {
  opacity: 0.3;
  cursor: default;
}

.counter-text {
  font-size: 0.875rem;
  color: #71717a;
  min-width: 80px;
  text-align: center;
}

.image-timeline {
  margin-top: 0.25rem;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  background: #111118;
  border: 1px solid #27272a;
  border-radius: 12px;
  padding: 0.75rem;
}

.timeline-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  color: #a1a1aa;
}

.timeline-count {
  margin-left: auto;
  font-size: 0.75rem;
  color: #52525b;
}

.timeline-track {
  display: flex;
  gap: 0.5rem;
  overflow-x: auto;
  padding-bottom: 0.25rem;
}

.timeline-thumb {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  min-width: 90px;
  border: 1px solid #27272a;
  background: #0a0a0f;
  border-radius: 10px;
  padding: 0.4rem;
  cursor: pointer;
  transition: all 0.2s;
}

.timeline-thumb img {
  width: 100%;
  height: 52px;
  object-fit: cover;
  border-radius: 6px;
  background: #000;
}

.timeline-thumb .thumb-label {
  font-size: 0.75rem;
  color: #a1a1aa;
  text-align: center;
}

.timeline-thumb.active {
  border-color: #8b5cf6;
  box-shadow: 0 0 0 1px rgba(139, 92, 246, 0.4);
}

.timeline-thumb:hover {
  border-color: #3b82f6;
}

/* Controls Panel */
.controls-panel {
  background: #111118;
  border-left: 1px solid #27272a;
  padding: 1.5rem;
  overflow-y: auto;
}

.controls-panel h3 {
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #71717a;
  margin-bottom: 1rem;
}

.label-grid {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.label-card {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.875rem 1rem;
  border: 1px solid #27272a;
  border-radius: 12px;
  background: #18181b;
  color: #e4e4e7;
  font-size: 0.9375rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
  text-align: left;
}

.label-card .mdi {
  font-size: 1.375rem;
  color: var(--accent-color);
}

.label-card:hover:not(:disabled) {
  border-color: var(--accent-color);
  background: rgba(139, 92, 246, 0.1);
}

.label-card.active {
  border-color: #22c55e;
  background: rgba(34, 197, 94, 0.1);
}

.label-card:disabled {
  opacity: 0.5;
  cursor: default;
}

.label-name {
  flex: 1;
}

.label-key {
  font-size: 0.75rem;
  color: #52525b;
  background: #27272a;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
}

.divider {
  height: 1px;
  background: #27272a;
  margin: 1.5rem 0;
}

.meteor-card {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  width: 100%;
  padding: 0.875rem 1rem;
  border: 1px dashed #fbbf24;
  border-radius: 12px;
  background: transparent;
  color: #fbbf24;
  font-size: 0.9375rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
  margin-bottom: 1.5rem;
}

.meteor-card .mdi {
  font-size: 1.375rem;
}

.meteor-card span:nth-child(2) {
  flex: 1;
  text-align: left;
}

.meteor-card:hover:not(:disabled) {
  background: rgba(251, 191, 36, 0.1);
}

.meteor-card:disabled {
  opacity: 0.5;
  cursor: default;
}

.shortcuts {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.shortcut {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.8125rem;
  color: #52525b;
}

.shortcut kbd {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 24px;
  height: 22px;
  padding: 0 0.375rem;
  background: #27272a;
  border-radius: 4px;
  font-family: inherit;
  font-size: 0.6875rem;
  color: #a1a1aa;
}

/* Classify View */
.classify-view {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: auto;
}

.classify-content {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 1rem;
  padding: 1.5rem 2rem;
}

.classify-card {
  background: #111118;
  border: 1px solid #27272a;
  border-radius: 14px;
  padding: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.classify-card-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
}

.classify-card-header h3 {
  margin: 0;
  color: #fafafa;
}

.loading-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: #a1a1aa;
}

.loading-row .mdi {
  font-size: 1.25rem;
  color: #8b5cf6;
}

.latest-result {
  display: grid;
  grid-template-columns: minmax(240px, 320px) 1fr;
  gap: 1rem;
  align-items: center;
}

.latest-image img {
  width: 100%;
  border-radius: 12px;
  object-fit: cover;
  background: #000;
  border: 1px solid #27272a;
}

.latest-meta h4 {
  margin: 0;
  color: #fafafa;
}

.prediction-badge {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0.75rem;
  background: rgba(99, 102, 241, 0.12);
  border: 1px solid rgba(99, 102, 241, 0.35);
  border-radius: 999px;
  color: #cdd5ff;
  width: fit-content;
}

.prediction-badge.subtle {
  background: rgba(255, 255, 255, 0.04);
  border-color: #27272a;
  color: #a1a1aa;
}

.pred-label {
  text-transform: capitalize;
  font-weight: 600;
}

.pred-confidence {
  color: #8b5cf6;
  font-weight: 600;
}

.prob-row {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  margin-top: 0.5rem;
}

.prob-item {
  display: grid;
  grid-template-columns: 120px 1fr 50px;
  gap: 0.5rem;
  align-items: center;
}

.prob-bar {
  height: 8px;
  background: #1f1f27;
  border-radius: 4px;
  overflow: hidden;
}

.prob-fill {
  height: 100%;
  background: linear-gradient(90deg, #8b5cf6, #6366f1);
}

.prob-value {
  text-align: right;
  color: #e4e4e7;
  font-size: 0.85rem;
}

.upload-drop {
  border: 1px dashed #3f3f46;
  border-radius: 12px;
  padding: 1rem;
  color: #c7d2fe;
  background: #0a0a10;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.upload-drop input {
  display: none;
}

.upload-preview {
  position: relative;
  margin-top: 0.75rem;
}

.upload-preview img {
  width: 100%;
  max-height: 280px;
  object-fit: contain;
  background: #000;
  border-radius: 12px;
  border: 1px solid #27272a;
}

.upload-preview .icon-btn {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
}

.upload-actions {
  display: flex;
  justify-content: flex-start;
}

.upload-result {
  margin-top: 0.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.empty-state.compact {
  padding: 1.25rem;
}

.empty-state.compact h2 {
  margin: 0.25rem 0;
}

@media (max-width: 900px) {
  .latest-result {
    grid-template-columns: 1fr;
  }
}

/* Train View */
.train-view {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: auto;
}

.train-content {
  flex: 1;
  padding: 2rem;
  overflow-y: auto;
  max-width: 800px;
}

.status-cards {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1rem;
  margin-bottom: 2rem;
}

.status-card {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1.25rem;
  background: #111118;
  border: 1px solid #27272a;
  border-radius: 16px;
}

.status-card .mdi {
  font-size: 1.75rem;
  color: #8b5cf6;
}

.status-card.status-running .mdi {
  color: #3b82f6;
}

.status-info {
  display: flex;
  flex-direction: column;
}

.status-value {
  font-size: 1.125rem;
  font-weight: 600;
  color: #fafafa;
}

.status-label {
  font-size: 0.8125rem;
  color: #71717a;
}

.warning-banner {
  display: flex;
  align-items: flex-start;
  gap: 1rem;
  padding: 1rem 1.25rem;
  background: rgba(251, 191, 36, 0.1);
  border: 1px solid rgba(251, 191, 36, 0.3);
  border-radius: 12px;
  margin-bottom: 2rem;
}

.warning-banner .mdi {
  font-size: 1.5rem;
  color: #fbbf24;
  flex-shrink: 0;
}

.warning-banner strong {
  color: #fbbf24;
}

.warning-banner p {
  margin-top: 0.25rem;
  font-size: 0.875rem;
  color: #a1a1aa;
}

.config-section {
  margin-bottom: 2rem;
}

.config-section h2 {
  font-size: 1.125rem;
  font-weight: 600;
  margin-bottom: 1.25rem;
}

.config-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1rem;
}

.config-field {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.config-field label {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  font-weight: 500;
  color: #a1a1aa;
}

.config-field label .mdi {
  font-size: 1rem;
  color: #71717a;
}

.config-field input {
  padding: 0.75rem 1rem;
  border: 1px solid #27272a;
  border-radius: 10px;
  background: #18181b;
  color: #fafafa;
  font-size: 1rem;
  transition: all 0.2s;
}

.config-field input:focus {
  outline: none;
  border-color: #8b5cf6;
}

.field-hint {
  font-size: 0.75rem;
  color: #52525b;
}

.scratch-toggle {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1rem 1.25rem;
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 12px;
  cursor: pointer;
  transition: all 0.2s;
  margin-top: 1rem;
}

.scratch-toggle:hover {
  border-color: #3f3f46;
}

.scratch-toggle input {
  display: none;
}

.scratch-toggle .mdi {
  font-size: 1.5rem;
  color: #8b5cf6;
}

.scratch-toggle input:checked ~ .mdi {
  color: #f59e0b;
}

.scratch-content {
  display: flex;
  flex-direction: column;
}

.scratch-label {
  font-weight: 500;
  color: #fafafa;
}

.scratch-hint {
  font-size: 0.8125rem;
  color: #71717a;
}

.scratch-mode-note {
  font-size: 0.75rem;
  color: #52525b;
}

.actions {
  margin-bottom: 1.5rem;
}

.btn-primary,
.btn-danger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 0.875rem 2rem;
  border: none;
  border-radius: 12px;
  font-size: 1rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-primary {
  background: linear-gradient(135deg, #8b5cf6, #6366f1);
  color: white;
}

.btn-primary:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 4px 20px rgba(139, 92, 246, 0.4);
}

.btn-primary:disabled {
  opacity: 0.5;
  cursor: default;
}

.btn-danger {
  background: #dc2626;
  color: white;
}

.btn-danger.ghost {
  background: transparent;
  border: 1px solid rgba(220, 38, 38, 0.5);
  color: #fca5a5;
  padding: 0.6rem 1rem;
}

.btn-danger:hover:not(:disabled) {
  background: #b91c1c;
}

.btn-danger:disabled {
  opacity: 0.5;
  cursor: default;
}

.danger-inline {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-right: 1rem;
}

.danger-hint {
  font-size: 0.75rem;
  color: #fca5a5;
}

.clear-message {
  color: #fca5a5;
  padding: 0 2rem;
  margin-top: 0.35rem;
  font-size: 0.85rem;
}

.btn-download {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.5em;
  padding: 0.75em 1.2em;
  background: linear-gradient(135deg, #2563eb, #7c3aed);
  color: #fff;
  border-radius: 10px;
  text-decoration: none;
  font-weight: 600;
  transition: background 0.2s, transform 0.2s;
  font-size: 1rem;
  border: 1px solid transparent;
}

.btn-download:hover {
  background: linear-gradient(135deg, #1d4ed8, #6d28d9);
  transform: translateY(-1px);
}

.download-section {
  margin-bottom: 1.5em;
}

.error-message {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 1rem;
  background: rgba(220, 38, 38, 0.1);
  border: 1px solid rgba(220, 38, 38, 0.3);
  border-radius: 10px;
  color: #f87171;
  margin-bottom: 1.5rem;
}

.error-message.small {
  margin-bottom: 0.5rem;
}

.error-message .mdi {
  font-size: 1.25rem;
}

.training-status,
.success-status,
.failed-status {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 1rem 1.25rem;
  border-radius: 12px;
  margin-bottom: 1.5rem;
}

.training-status {
  background: rgba(59, 130, 246, 0.1);
  border: 1px solid rgba(59, 130, 246, 0.3);
  flex-direction: column;
  align-items: flex-start;
}

.training-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  color: #60a5fa;
  font-weight: 500;
}

.training-time {
  font-size: 0.875rem;
  color: #71717a;
  margin-top: 0.25rem;
}

.success-status {
  background: rgba(34, 197, 94, 0.1);
  border: 1px solid rgba(34, 197, 94, 0.3);
  color: #4ade80;
}

.failed-status {
  background: rgba(220, 38, 38, 0.1);
  border: 1px solid rgba(220, 38, 38, 0.3);
  color: #f87171;
}

.logs-section {
  margin-top: 1.5rem;
}

.logs-section h3 {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.875rem;
  font-weight: 600;
  color: #a1a1aa;
  margin-bottom: 0.75rem;
}

.logs-content {
  padding: 1rem;
  background: #0a0a0f;
  border: 1px solid #27272a;
  border-radius: 10px;
  font-family: "JetBrains Mono", "Fira Code", monospace;
  font-size: 0.8125rem;
  line-height: 1.6;
  color: #a1a1aa;
  max-height: 400px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

/* Model Version List */
.model-list {
  margin-top: 1em;
}
.model-list h3 {
  font-size: 1em;
  color: #a1a1aa;
  margin-bottom: 0.5em;
}
.model-list ul {
  list-style: none;
  padding: 0;
  margin: 0;
}
.model-list li {
  display: flex;
  align-items: center;
  gap: 1em;
  margin-bottom: 0.5em;
}
.model-version {
  font-family: monospace;
  color: #8b5cf6;
  font-size: 1em;
}
.btn-small {
  font-size: 0.95em;
  padding: 0.45em 0.8em;
  margin-left: 0.5em;
  background: #111827;
  border: 1px solid #27272a;
}

/* Animations */
@keyframes spin {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}

.mdi-spin {
  animation: spin 1s linear infinite;
}

.model-card {
  background: #0f0f15;
  border: 1px solid #27272a;
  border-radius: 14px;
  padding: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.25);
}

.model-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.model-card-header h3 {
  margin: 0;
  font-size: 1.05rem;
  color: #fafafa;
}

.model-card-header .muted {
  color: #71717a;
  font-size: 0.875rem;
}

.pill {
  padding: 0.35rem 0.75rem;
  background: rgba(99, 102, 241, 0.15);
  border: 1px solid rgba(99, 102, 241, 0.3);
  border-radius: 999px;
  color: #c4c6ff;
  font-weight: 600;
  font-size: 0.875rem;
}

.model-controls {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 1rem;
  align-items: end;
}

.model-field {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  font-size: 0.9rem;
  color: #a1a1aa;
}

.model-field select {
  background: #111827;
  color: #fff;
  border: 1px solid #27272a;
  border-radius: 10px;
  padding: 0.65rem 0.75rem;
  font-size: 0.95rem;
}

.model-actions {
  display: flex;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.btn-download.wide {
  flex: 1;
  justify-content: center;
}

.btn-download.disabled {
  opacity: 0.5;
  pointer-events: none;
}

.btn-primary.ghost {
  background: transparent;
  border: 1px solid #3b82f6;
  color: #c7d2fe;
  padding: 0.75rem 1.5rem;
}

.model-action-message {
  color: #c7d2fe;
  font-size: 0.9rem;
}

.manual-card {
  margin: 1.5rem 0;
  padding: 1rem 1.25rem;
  background: #0f0f15;
  border: 1px solid #27272a;
  border-radius: 12px;
  color: #cdd5ff;
  line-height: 1.6;
}

.manual-card h3 {
  margin-bottom: 0.75rem;
  font-size: 1rem;
  color: #fafafa;
}

.manual-list {
  list-style: disc;
  padding-left: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
}

.manual-list strong {
  color: #c7d2fe;
}

.api-link {
  color: #7c3aed;
  text-decoration: underline;
}

.model-version-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem 0;
  border-bottom: 1px solid #1f1f2b;
}

.model-links {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.model-created {
  color: #565f7a;
  font-size: 0.85rem;
  margin-left: 0.4rem;
}

.info-note {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  padding: 0.85rem 1rem;
  background: rgba(34, 197, 94, 0.08);
  border: 1px solid rgba(34, 197, 94, 0.35);
  border-radius: 12px;
  color: #86efac;
  font-size: 0.95rem;
  margin-top: 0.5rem;
}

.info-note code {
  background: rgba(255, 255, 255, 0.06);
  padding: 0.15rem 0.4rem;
  border-radius: 6px;
  color: #e4e4e7;
}

/* Scrollbar */
::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: #27272a;
  border-radius: 4px;
}

::-webkit-scrollbar-thumb:hover {
  background: #3f3f46;
}
</style>
