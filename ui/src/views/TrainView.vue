<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed } from "vue";
import { useStore } from "../stores/useStore";

const store = useStore();

const epochs = ref(10);
const batchSize = ref(16);
const learningRate = ref("0.001");
const fromScratch = ref(true);
const loading = ref(false);
const error = ref("");
const selectedVersion = ref<string | null>(null);

const canTrain = computed(() => store.stats.value.labeled >= 10);

async function loadData() {
  await Promise.all([
    store.fetchTrainStatus(),
    store.fetchModelInfo(),
    store.fetchModelVersions(),
    store.fetchStats(),
  ]);
  if (store.modelVersions.value.length > 0) {
    selectedVersion.value = store.modelVersions.value[0]!.version;
  }
}

async function handleStart() {
  loading.value = true;
  error.value = "";
  const success = await store.startTraining({
    epochs: epochs.value,
    batch_size: batchSize.value,
    lr: learningRate.value,
    from_scratch: fromScratch.value,
  });
  if (!success) {
    error.value = "Failed to start training";
  }
  loading.value = false;
}

async function handleStop() {
  loading.value = true;
  await store.stopTraining();
  loading.value = false;
}

async function handleSwitchModel(version: string) {
  await store.switchModel(version);
  await store.fetchModelVersions();
}

let pollInterval: number | null = null;

onMounted(() => {
  loadData();
  pollInterval = window.setInterval(() => {
    store.fetchTrainStatus();
    store.fetchModelInfo();
  }, 3000);
});

onUnmounted(() => {
  if (pollInterval) clearInterval(pollInterval);
});
</script>

<template>
  <div class="train-view">
    <header class="header">
      <h1>Train Model</h1>
      <p class="subtitle">Train a new sky classification model</p>
    </header>

    <!-- Status Cards -->
    <div class="status-grid">
      <div class="status-card">
        <span class="mdi mdi-tag-check"></span>
        <div>
          <span class="status-value">{{ store.stats.value.labeled }}</span>
          <span class="status-label">Labeled Images</span>
        </div>
      </div>

      <div class="status-card">
        <span class="mdi mdi-cube-outline"></span>
        <div>
          <span class="status-value">{{
            store.modelInfo.value.active || "None"
          }}</span>
          <span class="status-label">Active Model</span>
        </div>
      </div>

      <div
        class="status-card"
        :class="{ running: store.trainStatus.value.running }"
      >
        <span
          class="mdi"
          :class="
            store.trainStatus.value.running
              ? 'mdi-loading mdi-spin'
              : 'mdi-check-circle'
          "
        ></span>
        <div>
          <span class="status-value">
            {{ store.trainStatus.value.running ? "Training..." : "Idle" }}
          </span>
          <span class="status-label">Status</span>
        </div>
      </div>
    </div>

    <!-- Warning -->
    <div v-if="!canTrain" class="warning">
      <span class="mdi mdi-alert"></span>
      <div>
        <strong>Not enough data</strong>
        <p>
          You need at least 10 labeled images. Currently:
          {{ store.stats.value.labeled }}
        </p>
      </div>
    </div>

    <!-- Config -->
    <section v-if="!store.trainStatus.value.running" class="config-section">
      <h2>Configuration</h2>
      <div class="config-grid">
        <div class="config-field">
          <label>Epochs</label>
          <input type="number" v-model="epochs" min="1" max="1000" />
        </div>
        <div class="config-field">
          <label>Batch Size</label>
          <input type="number" v-model="batchSize" min="1" max="256" />
        </div>
        <div class="config-field">
          <label>Learning Rate</label>
          <input type="text" v-model="learningRate" />
        </div>
      </div>

      <label class="scratch-option">
        <input type="checkbox" v-model="fromScratch" />
        <div class="scratch-content">
          <span
            class="mdi"
            :class="fromScratch ? 'mdi-restart' : 'mdi-school'"
          ></span>
          <div>
            <strong>{{
              fromScratch ? "Train from scratch" : "Continue learning"
            }}</strong>
            <p>
              {{
                fromScratch
                  ? "Reset weights and start fresh"
                  : "Fine-tune existing model"
              }}
            </p>
          </div>
        </div>
      </label>
    </section>

    <!-- Actions -->
    <div class="actions">
      <button
        v-if="!store.trainStatus.value.running"
        class="btn-primary"
        :disabled="loading || !canTrain"
        @click="handleStart"
      >
        <span class="mdi mdi-play"></span>
        {{ loading ? "Starting..." : "Start Training" }}
      </button>
      <button v-else class="btn-danger" :disabled="loading" @click="handleStop">
        <span class="mdi mdi-stop"></span>
        {{ loading ? "Stopping..." : "Stop Training" }}
      </button>
    </div>

    <!-- Error -->
    <div v-if="error" class="error">
      <span class="mdi mdi-alert-circle"></span>
      {{ error }}
    </div>

    <!-- Logs -->
    <section v-if="store.trainStatus.value.logs" class="logs-section">
      <h2>Training Logs</h2>
      <pre class="logs">{{ store.trainStatus.value.logs }}</pre>
    </section>

    <!-- Models -->
    <section class="models-section">
      <h2>Available Models</h2>
      <div v-if="store.modelVersions.value.length === 0" class="empty">
        No models trained yet
      </div>
      <div v-else class="model-list">
        <div
          v-for="m in store.modelVersions.value"
          :key="m.version"
          class="model-item"
          :class="{ active: store.modelInfo.value.active === m.version }"
        >
          <div class="model-info">
            <span class="model-version">{{ m.version }}</span>
            <span v-if="m.created_at" class="model-date">
              {{ new Date(m.created_at).toLocaleDateString() }}
            </span>
          </div>
          <div class="model-actions">
            <a
              v-if="m['model.onnx']"
              :href="m['model.onnx']"
              class="btn-small"
              download
            >
              <span class="mdi mdi-download"></span> ONNX
            </a>
            <a
              v-if="m['model.pt']"
              :href="m['model.pt']"
              class="btn-small"
              download
            >
              <span class="mdi mdi-download"></span> PT
            </a>
            <button
              v-if="store.modelInfo.value.active !== m.version"
              class="btn-small primary"
              @click="handleSwitchModel(m.version)"
            >
              Use
            </button>
            <span v-else class="badge">Active</span>
          </div>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.train-view {
  max-width: 900px;
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

.status-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 1rem;
  margin-bottom: 2rem;
}

.status-card {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1.25rem;
  background: #0f0f14;
  border: 1px solid #1f1f28;
  border-radius: 14px;
}

.status-card.running {
  border-color: rgba(99, 102, 241, 0.4);
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.08), transparent);
}

.status-card .mdi {
  font-size: 2rem;
  color: #6366f1;
}

.status-value {
  display: block;
  font-size: 1.25rem;
  font-weight: 700;
  color: #fafafa;
}

.status-label {
  font-size: 0.8rem;
  color: #71717a;
}

.warning {
  display: flex;
  align-items: flex-start;
  gap: 1rem;
  padding: 1rem 1.25rem;
  background: rgba(250, 204, 21, 0.1);
  border: 1px solid rgba(250, 204, 21, 0.3);
  border-radius: 12px;
  margin-bottom: 2rem;
}

.warning .mdi {
  font-size: 1.5rem;
  color: #facc15;
}

.warning strong {
  color: #fef08a;
}

.warning p {
  margin: 0.25rem 0 0;
  color: #a3a3a3;
  font-size: 0.9rem;
}

.config-section {
  background: #0f0f14;
  border: 1px solid #1f1f28;
  border-radius: 16px;
  padding: 1.5rem;
  margin-bottom: 1.5rem;
}

.config-section h2 {
  font-size: 1rem;
  font-weight: 600;
  color: #a1a1aa;
  margin: 0 0 1.25rem;
}

.config-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1rem;
  margin-bottom: 1.5rem;
}

@media (max-width: 600px) {
  .config-grid {
    grid-template-columns: 1fr;
  }
}

.config-field {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.config-field label {
  font-size: 0.85rem;
  color: #71717a;
}

.config-field input {
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 10px;
  padding: 0.75rem;
  color: #fff;
  font-size: 1rem;
}

.scratch-option {
  display: flex;
  align-items: flex-start;
  gap: 1rem;
  padding: 1rem;
  background: #18181b;
  border-radius: 12px;
  cursor: pointer;
}

.scratch-option input {
  margin-top: 0.25rem;
  accent-color: #6366f1;
  width: 18px;
  height: 18px;
}

.scratch-content {
  display: flex;
  gap: 0.75rem;
}

.scratch-content .mdi {
  font-size: 1.5rem;
  color: #6366f1;
}

.scratch-content strong {
  display: block;
  color: #fafafa;
}

.scratch-content p {
  margin: 0.25rem 0 0;
  font-size: 0.85rem;
  color: #71717a;
}

.actions {
  margin-bottom: 1.5rem;
}

.btn-primary,
.btn-danger {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.875rem 1.5rem;
  border: none;
  border-radius: 12px;
  font-size: 1rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-primary {
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  color: #fff;
}

.btn-primary:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow: 0 8px 20px rgba(99, 102, 241, 0.3);
}

.btn-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.btn-danger {
  background: #dc2626;
  color: #fff;
}

.btn-danger:hover:not(:disabled) {
  background: #ef4444;
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
  margin-bottom: 1.5rem;
}

.logs-section {
  margin-bottom: 2rem;
}

.logs-section h2 {
  font-size: 1rem;
  font-weight: 600;
  color: #a1a1aa;
  margin-bottom: 1rem;
}

.logs {
  background: #0a0a0f;
  border: 1px solid #1f1f28;
  border-radius: 12px;
  padding: 1rem;
  font-family: "JetBrains Mono", monospace;
  font-size: 0.8rem;
  color: #a1a1aa;
  max-height: 300px;
  overflow: auto;
  white-space: pre-wrap;
}

.models-section h2 {
  font-size: 1rem;
  font-weight: 600;
  color: #a1a1aa;
  margin-bottom: 1rem;
}

.empty {
  color: #52525b;
  text-align: center;
  padding: 2rem;
}

.model-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.model-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 1rem 1.25rem;
  background: #0f0f14;
  border: 1px solid #1f1f28;
  border-radius: 12px;
}

.model-item.active {
  border-color: rgba(99, 102, 241, 0.4);
}

.model-version {
  font-family: monospace;
  font-weight: 600;
  color: #c7d2fe;
}

.model-date {
  margin-left: 0.75rem;
  font-size: 0.85rem;
  color: #52525b;
}

.model-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.btn-small {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.5rem 0.75rem;
  background: #18181b;
  border: 1px solid #27272a;
  border-radius: 8px;
  color: #a1a1aa;
  font-size: 0.8rem;
  text-decoration: none;
  cursor: pointer;
}

.btn-small:hover {
  background: #27272a;
  color: #fff;
}

.btn-small.primary {
  background: rgba(99, 102, 241, 0.15);
  border-color: rgba(99, 102, 241, 0.3);
  color: #c7d2fe;
}

.badge {
  padding: 0.35rem 0.75rem;
  background: rgba(34, 197, 94, 0.15);
  border: 1px solid rgba(34, 197, 94, 0.3);
  border-radius: 999px;
  font-size: 0.75rem;
  color: #86efac;
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
