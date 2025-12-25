import { ref, computed } from "vue";

// ============ Types ============
export interface ImageItem {
  id: string;
  path: string;
  sha256: string;
  fetched_at: string;
  size_bytes?: number;
  skystate?: string;
  meteor?: boolean;
  labeled_at?: string;
}

export interface DaySummary {
  date: string;
  count: number;
  size_bytes?: number;
}

export interface DatasetStats {
  total: number;
  labeled: number;
  unlabeled: number;
  by_class: Record<string, number>;
  total_size_bytes: number;
}

export interface TrainStatus {
  running: boolean;
  container_id?: string;
  started_at?: string;
  exit_code?: number;
  error?: string;
  logs?: string;
}

export interface ModelInfo {
  active: string | null;
  path?: string;
}

export interface ModelVersion {
  version: string;
  created_at?: string;
  "model.onnx"?: string;
  "model.pt"?: string;
}

export interface Prediction {
  skystate: string;
  confidence?: number;
  probs?: Record<string, number>;
}

// ============ State ============
const images = ref<ImageItem[]>([]);
const currentIndex = ref(0);
const days = ref<DaySummary[]>([]);
const selectedDay = ref<string | null>(null);
const stats = ref<DatasetStats>({
  total: 0,
  labeled: 0,
  unlabeled: 0,
  by_class: {},
  total_size_bytes: 0,
});
const trainStatus = ref<TrainStatus>({ running: false });
const modelInfo = ref<ModelInfo>({ active: null });
const modelVersions = ref<ModelVersion[]>([]);
const loading = ref(false);

// ============ Computed ============
const currentImage = computed(() => images.value[currentIndex.value] || null);
const hasNext = computed(() => currentIndex.value < images.value.length - 1);
const hasPrev = computed(() => currentIndex.value > 0);
const progress = computed(() => {
  const { labeled, unlabeled } = stats.value;
  const total = labeled + unlabeled;
  return total > 0 ? Math.round((labeled / total) * 100) : 0;
});

// ============ API Functions ============
async function fetchImages(
  options: {
    unlabeled?: boolean;
    day?: string | null;
    limit?: number;
  } = {}
) {
  const params = new URLSearchParams();
  if (options.unlabeled) params.set("unlabeled", "1");
  if (options.day) params.set("date", options.day);
  if (options.limit) params.set("limit", String(options.limit));

  const query = params.toString();
  const res = await fetch(`/api/dataset/images${query ? `?${query}` : ""}`);
  if (res.ok) {
    const data = await res.json();
    images.value = data.items || [];
    if (currentIndex.value >= images.value.length) {
      currentIndex.value = Math.max(0, images.value.length - 1);
    }
  }
}

async function fetchDays() {
  const res = await fetch("/api/dataset/days");
  if (res.ok) {
    const data = await res.json();
    days.value = data.days || [];
    if (!selectedDay.value && days.value.length > 0) {
      selectedDay.value = days.value[0]!.date;
    }
  }
}

async function fetchStats() {
  const res = await fetch("/api/dataset/stats");
  if (res.ok) {
    stats.value = await res.json();
  }
}

async function setLabel(imageId: string, skystate: string, meteor = false) {
  const res = await fetch("/api/labels", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ image_id: imageId, skystate, meteor }),
  });
  return res.ok;
}

async function fetchTrainStatus() {
  const res = await fetch("/api/train/status");
  if (res.ok) {
    trainStatus.value = await res.json();
  }
}

async function startTraining(config: {
  epochs: number;
  batch_size: number;
  lr: string;
  from_scratch: boolean;
}) {
  const res = await fetch("/api/train/start", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      ...config,
      img_size: 224,
      seed: 42,
      val_split: "0.2",
    }),
  });
  if (res.ok) {
    await fetchTrainStatus();
  }
  return res.ok;
}

async function stopTraining() {
  const res = await fetch("/api/train/stop", { method: "POST" });
  if (res.ok) {
    await fetchTrainStatus();
  }
  return res.ok;
}

async function fetchModelInfo() {
  const res = await fetch("/api/models");
  if (res.ok) {
    modelInfo.value = await res.json();
  }
}

async function fetchModelVersions() {
  const res = await fetch("/api/models/list");
  if (res.ok) {
    modelVersions.value = await res.json();
  }
}

async function switchModel(version: string) {
  const res = await fetch(
    `/api/models/reload?version=${encodeURIComponent(version)}`,
    {
      method: "POST",
    }
  );
  if (res.ok) {
    await fetchModelInfo();
  }
  return res.ok;
}

async function classifyImage(file: File): Promise<Prediction | null> {
  const form = new FormData();
  form.append("file", file);
  const res = await fetch("/api/classify", { method: "POST", body: form });
  if (res.ok) {
    const data = await res.json();
    return data.prediction;
  }
  return null;
}

async function cleanupByDay(day: string) {
  const res = await fetch(`/api/images/cleanup?day=${day}`, { method: "POST" });
  if (res.ok) {
    return await res.json();
  }
  return null;
}

// ============ Helpers ============
function formatBytes(bytes?: number | null): string {
  if (!bytes || bytes < 0) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let idx = 0;
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024;
    idx++;
  }
  return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[idx]}`;
}

function nextImage() {
  if (hasNext.value) currentIndex.value++;
}

function prevImage() {
  if (hasPrev.value) currentIndex.value--;
}

function setIndex(idx: number) {
  if (idx >= 0 && idx < images.value.length) {
    currentIndex.value = idx;
  }
}

// ============ Export ============
export function useStore() {
  return {
    // State
    images,
    currentIndex,
    days,
    selectedDay,
    stats,
    trainStatus,
    modelInfo,
    modelVersions,
    loading,
    // Computed
    currentImage,
    hasNext,
    hasPrev,
    progress,
    // API
    fetchImages,
    fetchDays,
    fetchStats,
    setLabel,
    fetchTrainStatus,
    startTraining,
    stopTraining,
    fetchModelInfo,
    fetchModelVersions,
    switchModel,
    classifyImage,
    cleanupByDay,
    // Helpers
    formatBytes,
    nextImage,
    prevImage,
    setIndex,
  };
}
